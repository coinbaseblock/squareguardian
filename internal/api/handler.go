package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"squareguardian/internal/detector"
)

// Handler serves the SpaceGuardian HTTP API.
type Handler struct {
	det *detector.Detector
	mux *http.ServeMux
}

// New creates a new API handler.
func New(det *detector.Detector) *Handler {
	h := &Handler{det: det, mux: http.NewServeMux()}
	h.mux.HandleFunc("/", h.dashboard)
	h.mux.HandleFunc("/events", h.eventsPage)
	h.mux.HandleFunc("/healthz", h.healthz)
	h.mux.HandleFunc("/api/events", h.events)
	h.mux.HandleFunc("/api/status", h.status)
	h.mux.HandleFunc("/api/annotate", h.annotate)
	h.mux.HandleFunc("/api/thumbnail/", h.thumbnail)
	h.mux.HandleFunc("/api/snapshot/", h.snapshot)
	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	events := h.det.Events("")
	labels := h.det.TrackedLabels()

	// Count events by label
	counts := make(map[string]int)
	for _, e := range events {
		counts[e.Label]++
	}

	// Build label summary rows
	labelRows := ""
	for _, l := range labels {
		c := counts[l]
		dot := "🔴"
		if c > 0 {
			dot = "🟢"
		}
		labelRows += fmt.Sprintf(
			`<tr><td>%s %s</td><td style="text-align:right;font-weight:bold">%d</td></tr>`,
			dot, l, c)
	}

	// Build recent events rows (last 20)
	limit := 20
	if len(events) < limit {
		limit = len(events)
	}
	eventRows := ""
	for _, e := range events[:limit] {
		eventRows += h.buildEventRow(e)
	}
	if eventRows == "" {
		eventRows = `<tr><td colspan="14" style="text-align:center;color:#888;padding:2em">ยังไม่มี event — รอ Frigate ตรวจจับ...</td></tr>`
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, dashboardTpl, len(events), len(labels), labelRows, eventRows)
}

func (h *Handler) eventsPage(w http.ResponseWriter, r *http.Request) {
	events := h.det.Events("")
	labels := h.det.TrackedLabels()

	// Sort labels for consistent display
	sort.Strings(labels)

	// Group events by label
	grouped := make(map[string][]detector.Event)
	for _, e := range events {
		grouped[e.Label] = append(grouped[e.Label], e)
	}

	// Build sections for each label
	sections := ""
	for _, label := range labels {
		evts := grouped[label]
		if len(evts) == 0 {
			continue
		}

		// Display label name in Thai where applicable
		displayName := labelDisplayName(label)

		// Build thumbnail grid
		thumbs := ""
		showLimit := 50
		if len(evts) < showLimit {
			showLimit = len(evts)
		}
		for _, e := range evts[:showLimit] {
			t := time.Unix(int64(e.StartTime), int64(math.Mod(e.StartTime, 1)*1e9))
			ts := t.Format("15:04")
			ago := timeAgo(e.StartTime)

			identBadge := ""
			if e.Identity != "" {
				identBadge = fmt.Sprintf(`<span class="ident-badge">%s</span>`, e.Identity)
			}
			if e.RoomNumber != "" {
				identBadge += fmt.Sprintf(`<span class="room-badge">%s</span>`, e.RoomNumber)
			}

			var thumbSrc string
			if e.Thumbnail != "" {
				thumbSrc = fmt.Sprintf("data:image/jpeg;base64,%s", e.Thumbnail)
			} else {
				thumbSrc = fmt.Sprintf("/api/thumbnail/%s", e.ID)
			}

			thumbs += fmt.Sprintf(`<div class="ev-card" onclick="openFeedback('%s','%s','%s','%s','%s','%s','%s','%s','%s')">
				<img src="%s" alt="%s" loading="lazy">
				<div class="ev-time">%s · %s</div>
				%s
			</div>`,
				e.ID, e.Label,
				escapeJS(e.Identity), escapeJS(e.RoomNumber),
				escapeJS(e.LicensePlate), escapeJS(e.Province),
				escapeJS(e.VehicleBrand), escapeJS(e.VehicleColor),
				escapeJS(e.Note),
				thumbSrc, e.Label,
				ts, ago,
				identBadge)
		}

		moreIndicator := ""
		if len(evts) > showLimit {
			moreIndicator = fmt.Sprintf(`<div class="more-indicator">+%d รายการ</div>`, len(evts)-showLimit)
		}

		sections += fmt.Sprintf(`<div class="ev-section">
			<h2>%s <span class="ev-count">%d Tracked Objects</span></h2>
			<div class="ev-grid">%s%s</div>
		</div>`, displayName, len(evts), thumbs, moreIndicator)
	}

	if sections == "" {
		sections = `<div style="text-align:center;color:#888;padding:3em">ยังไม่มี event — รอ Frigate ตรวจจับ...</div>`
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, eventsPageTpl, sections)
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	label := r.URL.Query().Get("label")
	events := h.det.Events(label)
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(events),
		"events": events,
	})
}

func (h *Handler) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":        "squareguardian",
		"tracked_labels": h.det.TrackedLabels(),
		"cached_events":  len(h.det.Events("")),
	})
}

func (h *Handler) annotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		EventID      string `json:"event_id"`
		Identity     string `json:"identity"`
		RoomNumber   string `json:"room_number"`
		LicensePlate string `json:"license_plate"`
		Province     string `json:"province"`
		VehicleBrand string `json:"vehicle_brand"`
		VehicleColor string `json:"vehicle_color"`
		VehicleInfo  string `json:"vehicle_info"`
		Note         string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.EventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event_id required"})
		return
	}

	if h.det.Annotate(req.EventID, req.Identity, req.RoomNumber, req.LicensePlate, req.Province, req.VehicleBrand, req.VehicleColor, req.VehicleInfo, req.Note) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	} else {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
	}
}

func (h *Handler) thumbnail(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimPrefix(r.URL.Path, "/api/thumbnail/")
	if eventID == "" {
		http.Error(w, "event_id required", http.StatusBadRequest)
		return
	}

	// Try serving from cached base64 thumbnail data first
	for _, e := range h.det.Events("") {
		if e.ID == eventID && e.Thumbnail != "" {
			data, err := base64.StdEncoding.DecodeString(e.Thumbnail)
			if err == nil {
				w.Header().Set("Content-Type", "image/jpeg")
				w.Header().Set("Cache-Control", "public, max-age=60")
				w.Write(data)
				return
			}
			break
		}
	}

	// Fallback: proxy to Frigate
	h.proxyFrigate(w, fmt.Sprintf("/api/events/%s/thumbnail.jpg", eventID))
}

func (h *Handler) snapshot(w http.ResponseWriter, r *http.Request) {
	eventID := strings.TrimPrefix(r.URL.Path, "/api/snapshot/")
	if eventID == "" {
		http.Error(w, "event_id required", http.StatusBadRequest)
		return
	}
	h.proxyFrigate(w, fmt.Sprintf("/api/events/%s/snapshot.jpg", eventID))
}

func (h *Handler) proxyFrigate(w http.ResponseWriter, path string) {
	url := h.det.FrigateURL() + path
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *Handler) buildEventRow(e detector.Event) string {
	t := time.Unix(int64(e.StartTime), int64(math.Mod(e.StartTime, 1)*1e9))
	ts := t.Format("2006-01-02 15:04:05")
	score := fmt.Sprintf("%.0f%%", e.TopScore*100)
	zone := e.Zone
	if zone == "" {
		zone = "-"
	}

	subLabel := e.SubLabel
	if subLabel == "" {
		subLabel = "-"
	}

	identityDisplay := e.Identity
	if identityDisplay == "" {
		identityDisplay = "-"
	}
	roomDisplay := e.RoomNumber
	if roomDisplay == "" {
		roomDisplay = "-"
	}
	plateDisplay := e.LicensePlate
	if e.Province != "" {
		plateDisplay += " " + e.Province
	}
	if plateDisplay == "" {
		plateDisplay = "-"
	}
	vehicleDisplay := ""
	if e.VehicleBrand != "" {
		vehicleDisplay = e.VehicleBrand
	}
	if e.VehicleColor != "" {
		if vehicleDisplay != "" {
			vehicleDisplay += " "
		}
		vehicleDisplay += e.VehicleColor
	}
	if vehicleDisplay == "" {
		vehicleDisplay = e.VehicleInfo
	}
	if vehicleDisplay == "" {
		vehicleDisplay = "-"
	}
	noteDisplay := e.Note

	thumbImg := ""
	if e.Thumbnail != "" {
		thumbImg = fmt.Sprintf(
			`<img src="data:image/jpeg;base64,%s" alt="%s" style="width:80px;height:60px;object-fit:cover;border-radius:4px;background:#252836" loading="lazy">`,
			e.Thumbnail, e.Label)
	} else {
		thumbImg = fmt.Sprintf(
			`<img src="/api/thumbnail/%s" alt="%s" style="width:80px;height:60px;object-fit:cover;border-radius:4px;background:#252836" loading="lazy">`,
			e.ID, e.Label)
	}

	snapLink := ""
	if e.Snapshot != "" {
		snapLink = fmt.Sprintf(`<a href="/api/snapshot/%s" target="_blank">📷</a>`, e.ID)
	}

	feedbackBtn := fmt.Sprintf(
		`<button class="fb-btn" onclick="openFeedback('%s','%s','%s','%s','%s','%s','%s','%s','%s')">✏️</button>`,
		e.ID, e.Label,
		escapeJS(e.Identity), escapeJS(e.RoomNumber),
		escapeJS(e.LicensePlate), escapeJS(e.Province),
		escapeJS(e.VehicleBrand), escapeJS(e.VehicleColor),
		escapeJS(e.Note))

	return fmt.Sprintf(
		`<tr>
			<td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td>
			<td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td>
			<td class="note-cell">%s</td>
			<td>%s %s</td>
		</tr>`,
		thumbImg, ts, e.Camera, e.Label, subLabel, score, zone,
		identityDisplay, roomDisplay, plateDisplay, vehicleDisplay, noteDisplay,
		snapLink, feedbackBtn)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: json encode error: %v", err)
	}
}

func escapeJS(s string) string {
	out := ""
	for _, c := range s {
		switch c {
		case '\'':
			out += "\\'"
		case '"':
			out += "\\&quot;"
		case '\\':
			out += "\\\\"
		case '\n':
			out += "\\n"
		default:
			out += string(c)
		}
	}
	return out
}

func labelDisplayName(label string) string {
	names := map[string]string{
		"person":     "Person (บุคคล)",
		"car":        "Car (รถยนต์)",
		"motorcycle": "Motorcycle (จักรยานยนต์)",
		"bus":        "Bus (รถบัส)",
		"truck":      "Truck (รถบรรทุก)",
		"backpack":   "Backpack (กระเป๋าเป้)",
		"suitcase":   "Suitcase (กระเป๋าเดินทาง)",
		"handbag":    "Handbag (กระเป๋าถือ)",
	}
	if n, ok := names[label]; ok {
		return n
	}
	return label
}

func timeAgo(startTime float64) string {
	d := time.Since(time.Unix(int64(startTime), 0))
	switch {
	case d < time.Minute:
		return "เมื่อสักครู่"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// feedbackModalHTML is shared between dashboard and events page
const feedbackModalHTML = `
<!-- Feedback Modal -->
<div class="modal-bg" id="feedbackModal">
<div class="modal">
  <h3>ระบุข้อมูลเพิ่มเติม</h3>
  <input type="hidden" id="fb-event-id">
  <p style="color:#888;font-size:.85em">Event: <span id="fb-label" style="color:#4fc3f7"></span></p>

  <div class="form-row">
    <div class="form-col">
      <label>ระบุตัวตน (ชื่อคน)</label>
      <input type="text" id="fb-identity" placeholder="เช่น สมชาย, คนส่งของ">
    </div>
    <div class="form-col">
      <label>เลขห้อง / Unit</label>
      <input type="text" id="fb-room" placeholder="เช่น A0213, B1502">
    </div>
  </div>

  <div class="form-row">
    <div class="form-col">
      <label>ทะเบียนรถ</label>
      <input type="text" id="fb-plate" placeholder="เช่น ทร 3474, สวย 123">
    </div>
    <div class="form-col">
      <label>จังหวัด (ถ้ามี)</label>
      <input type="text" id="fb-province" placeholder="เช่น กรุงเทพ, นนทบุรี">
    </div>
  </div>

  <div class="form-row">
    <div class="form-col">
      <label>ยี่ห้อ / รุ่น</label>
      <input type="text" id="fb-brand" placeholder="เช่น Toyota Camry, Honda Civic">
    </div>
    <div class="form-col">
      <label>สีรถ</label>
      <input type="text" id="fb-color" placeholder="เช่น สีขาว, สีดำ, สีแดง">
    </div>
  </div>

  <label>หมายเหตุ / Feedback</label>
  <textarea id="fb-note" placeholder="เช่น ตรวจจับถูกต้อง, เป็นคนส่งของ, รถของห้อง A0213"></textarea>

  <div class="actions">
    <button class="btn-cancel" onclick="closeFeedback()">ยกเลิก</button>
    <button class="btn-save" onclick="saveFeedback()">บันทึก</button>
  </div>
  <div class="msg" id="fb-msg"></div>
</div>
</div>`

const feedbackScript = `
<script>
function openFeedback(eventId, label, identity, room, plate, province, brand, color, note) {
  document.getElementById('fb-event-id').value = eventId;
  document.getElementById('fb-label').textContent = label + ' (' + eventId.substring(0,8) + '...)';
  document.getElementById('fb-identity').value = identity || '';
  document.getElementById('fb-room').value = room || '';
  document.getElementById('fb-plate').value = plate || '';
  document.getElementById('fb-province').value = province || '';
  document.getElementById('fb-brand').value = brand || '';
  document.getElementById('fb-color').value = color || '';
  document.getElementById('fb-note').value = note || '';
  document.getElementById('fb-msg').textContent = '';
  document.getElementById('feedbackModal').classList.add('open');
}

function closeFeedback() {
  document.getElementById('feedbackModal').classList.remove('open');
}

function saveFeedback() {
  var payload = {
    event_id: document.getElementById('fb-event-id').value,
    identity: document.getElementById('fb-identity').value,
    room_number: document.getElementById('fb-room').value,
    license_plate: document.getElementById('fb-plate').value,
    province: document.getElementById('fb-province').value,
    vehicle_brand: document.getElementById('fb-brand').value,
    vehicle_color: document.getElementById('fb-color').value,
    note: document.getElementById('fb-note').value
  };
  fetch('/api/annotate', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload)
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.status === 'ok') {
      document.getElementById('fb-msg').textContent = 'บันทึกสำเร็จ!';
      setTimeout(function() { closeFeedback(); location.reload(); }, 800);
    } else {
      document.getElementById('fb-msg').style.color = '#f44336';
      document.getElementById('fb-msg').textContent = 'Error: ' + (data.error || 'unknown');
    }
  })
  .catch(function(err) {
    document.getElementById('fb-msg').style.color = '#f44336';
    document.getElementById('fb-msg').textContent = 'Network error';
  });
}

// Close modal on background click
document.getElementById('feedbackModal').addEventListener('click', function(e) {
  if (e.target === this) closeFeedback();
});
</script>`

const sharedStyles = `
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;background:#0f1117;color:#e0e0e0;padding:1em}
h1{font-size:1.5em;margin-bottom:.5em;color:#fff}
.cards{display:flex;gap:1em;flex-wrap:wrap;margin-bottom:1.5em}
.card{background:#1a1d28;border-radius:8px;padding:1em 1.5em;min-width:140px;flex:1}
.card .num{font-size:2em;font-weight:bold;color:#4fc3f7}
.card .lbl{color:#999;font-size:.85em}
table{width:100%%;border-collapse:collapse;background:#1a1d28;border-radius:8px;overflow:hidden}
th{background:#252836;text-align:left;padding:.6em 1em;color:#999;font-size:.85em}
td{padding:.5em 1em;border-top:1px solid #252836;font-size:.85em}
tr:hover{background:#252836}
a{color:#4fc3f7;text-decoration:none}
.section{margin-bottom:1.5em}
.section h2{font-size:1.1em;margin-bottom:.5em;color:#ccc}
.status{display:inline-block;padding:2px 10px;border-radius:12px;font-size:.8em;font-weight:bold}
.status.ok{background:#1b5e20;color:#a5d6a7}
.links{margin-top:1em;display:flex;gap:1em;flex-wrap:wrap}
.links a{background:#252836;padding:.5em 1em;border-radius:6px;color:#4fc3f7}
.fb-btn{background:#333;border:1px solid #555;color:#4fc3f7;padding:2px 8px;border-radius:4px;cursor:pointer;font-size:.9em}
.fb-btn:hover{background:#444}
.note-cell{max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#aaa;font-size:.8em}

/* Modal */
.modal-bg{display:none;position:fixed;top:0;left:0;width:100%%;height:100%%;background:rgba(0,0,0,.7);z-index:100;justify-content:center;align-items:center}
.modal-bg.open{display:flex}
.modal{background:#1a1d28;border:1px solid #333;border-radius:12px;padding:1.5em;width:520px;max-width:95vw}
.modal h3{margin-bottom:1em;color:#fff}
.modal label{display:block;color:#999;font-size:.85em;margin-top:.8em}
.modal input,.modal textarea{width:100%%;padding:.5em;background:#252836;border:1px solid #444;border-radius:6px;color:#e0e0e0;font-size:.9em;margin-top:.3em}
.modal textarea{height:60px;resize:vertical}
.modal .actions{margin-top:1.2em;display:flex;gap:.8em;justify-content:flex-end}
.modal button{padding:.5em 1.2em;border-radius:6px;border:none;cursor:pointer;font-size:.9em}
.modal .btn-save{background:#1976d2;color:#fff}
.modal .btn-save:hover{background:#1565c0}
.modal .btn-cancel{background:#333;color:#ccc}
.modal .btn-cancel:hover{background:#444}
.modal .msg{margin-top:.5em;font-size:.85em;color:#4caf50}
.form-row{display:flex;gap:1em}
.form-col{flex:1}
.nav{display:flex;gap:1em;margin-bottom:1.5em}
.nav a{background:#252836;padding:.5em 1.2em;border-radius:6px;color:#4fc3f7;font-size:.9em}
.nav a.active{background:#1976d2;color:#fff}
`

var dashboardTpl = `<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SpaceGuardian — Dashboard</title>
<meta http-equiv="refresh" content="10">
<style>` + sharedStyles + `</style>
</head>
<body>
<h1>SpaceGuardian Dashboard</h1>
<div class="nav">
  <a href="/" class="active">Dashboard</a>
  <a href="/events">Events (แยกประเภท)</a>
</div>

<div class="cards">
  <div class="card"><div class="num">%d</div><div class="lbl">Events ทั้งหมด</div></div>
  <div class="card"><div class="num">%d</div><div class="lbl">ประเภทที่ติดตาม</div></div>
  <div class="card"><div class="num"><span class="status ok">ONLINE</span></div><div class="lbl">สถานะระบบ</div></div>
</div>

<div class="section">
<h2>สรุปตามประเภท</h2>
<table>
<tr><th>ประเภท</th><th style="text-align:right">จำนวน Event</th></tr>
%s
</table>
</div>

<div class="section">
<h2>Events ล่าสุด (20 รายการ)</h2>
<table>
<tr>
  <th>ภาพ</th><th>เวลา</th><th>กล้อง</th><th>ตรวจพบ</th><th>Sub-Label</th><th>ความมั่นใจ</th>
  <th>โซน</th><th>ระบุตัวตน</th><th>ห้อง</th><th>ทะเบียน</th><th>ยี่ห้อ/สี</th><th>หมายเหตุ</th><th></th>
</tr>
%s
</table>
</div>

` + feedbackModalHTML + `

<div class="links">
  <a href="/api/events">API: Events</a>
  <a href="/api/status">API: Status</a>
  <a href="/healthz">Health Check</a>
</div>
<p style="margin-top:1em;color:#666;font-size:.8em">Auto-refresh ทุก 10 วินาที | Storage: ไม่จำกัด (max 256 GB)</p>

` + feedbackScript + `
</body>
</html>`

var eventsPageTpl = `<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SpaceGuardian — Events</title>
<meta http-equiv="refresh" content="15">
<style>` + sharedStyles + `
/* Events page grid */
.ev-section{margin-bottom:2em}
.ev-section h2{font-size:1.2em;color:#fff;margin-bottom:.5em}
.ev-count{font-size:.85em;color:#888;font-weight:normal;margin-left:.5em}
.ev-grid{display:flex;gap:.6em;flex-wrap:wrap;padding:.5em 0}
.ev-card{position:relative;cursor:pointer;border-radius:8px;overflow:hidden;background:#1a1d28;width:150px;transition:transform .15s}
.ev-card:hover{transform:scale(1.05);z-index:2;box-shadow:0 4px 20px rgba(0,0,0,.5)}
.ev-card img{width:150px;height:110px;object-fit:cover;display:block}
.ev-time{position:absolute;bottom:0;left:0;right:0;background:linear-gradient(transparent,rgba(0,0,0,.8));color:#ddd;font-size:.7em;padding:4px 6px;text-align:right}
.ident-badge{position:absolute;top:4px;left:4px;background:rgba(25,118,210,.85);color:#fff;font-size:.65em;padding:2px 6px;border-radius:4px}
.room-badge{position:absolute;top:4px;right:4px;background:rgba(56,142,60,.85);color:#fff;font-size:.65em;padding:2px 6px;border-radius:4px}
.more-indicator{display:flex;align-items:center;justify-content:center;width:150px;height:110px;background:#252836;border-radius:8px;color:#4fc3f7;font-size:.85em;cursor:default}
</style>
</head>
<body>
<h1>SpaceGuardian — Events แยกประเภท</h1>
<div class="nav">
  <a href="/">Dashboard</a>
  <a href="/events" class="active">Events (แยกประเภท)</a>
</div>

<p style="color:#888;font-size:.85em;margin-bottom:1em">คลิกที่รูปเพื่อระบุตัวตน, ห้อง, ทะเบียนรถ, ยี่ห้อ/สี — ข้อมูลจะถูกใช้แยกแยะบุคคลและรถแต่ละคัน</p>

%s

` + feedbackModalHTML + `

<div class="links" style="margin-top:2em">
  <a href="/">Dashboard</a>
  <a href="/api/events">API: Events</a>
  <a href="/api/status">API: Status</a>
</div>
<p style="margin-top:1em;color:#666;font-size:.8em">Auto-refresh ทุก 15 วินาที | คลิกที่ภาพเพื่อระบุข้อมูล</p>

` + feedbackScript + `
</body>
</html>`
