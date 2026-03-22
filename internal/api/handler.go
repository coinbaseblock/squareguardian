package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
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
	h.mux.HandleFunc("/healthz", h.healthz)
	h.mux.HandleFunc("/api/events", h.events)
	h.mux.HandleFunc("/api/status", h.status)
	h.mux.HandleFunc("/api/annotate", h.annotate)
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
		t := time.Unix(int64(e.StartTime), int64(math.Mod(e.StartTime, 1)*1e9))
		ts := t.Format("2006-01-02 15:04:05")
		score := fmt.Sprintf("%.0f%%", e.TopScore*100)
		zone := e.Zone
		if zone == "" {
			zone = "-"
		}

		// Sub-label display
		subLabel := e.SubLabel
		if subLabel == "" {
			subLabel = "-"
		}

		// Identity / vehicle info
		identityDisplay := e.Identity
		if identityDisplay == "" {
			identityDisplay = "-"
		}
		vehicleDisplay := e.VehicleInfo
		if vehicleDisplay == "" {
			vehicleDisplay = "-"
		}
		noteDisplay := e.Note
		if noteDisplay == "" {
			noteDisplay = ""
		}

		snapLink := ""
		if e.Snapshot != "" {
			snapLink = fmt.Sprintf(`<a href="/api/events?label=%s" target="_blank">📷</a>`, e.Label)
		}

		// Feedback button
		feedbackBtn := fmt.Sprintf(
			`<button class="fb-btn" onclick="openFeedback('%s','%s','%s','%s','%s')">✏️</button>`,
			e.ID, e.Label, escapeJS(e.Identity), escapeJS(e.VehicleInfo), escapeJS(e.Note))

		eventRows += fmt.Sprintf(
			`<tr>
				<td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td>
				<td>%s</td><td>%s</td><td>%s</td>
				<td class="note-cell">%s</td>
				<td>%s %s</td>
			</tr>`,
			ts, e.Camera, e.Label, subLabel, score, zone,
			identityDisplay, vehicleDisplay, noteDisplay,
			snapLink, feedbackBtn)
	}
	if eventRows == "" {
		eventRows = `<tr><td colspan="10" style="text-align:center;color:#888;padding:2em">ยังไม่มี event — รอ Frigate ตรวจจับ...</td></tr>`
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, pageTpl, len(events), len(labels), labelRows, eventRows)
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
		EventID     string `json:"event_id"`
		Identity    string `json:"identity"`
		VehicleInfo string `json:"vehicle_info"`
		Note        string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.EventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event_id required"})
		return
	}

	if h.det.Annotate(req.EventID, req.Identity, req.VehicleInfo, req.Note) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	} else {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
	}
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

const pageTpl = `<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SpaceGuardian — Dashboard</title>
<meta http-equiv="refresh" content="10">
<style>
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
.links{margin-top:1em;display:flex;gap:1em}
.links a{background:#252836;padding:.5em 1em;border-radius:6px;color:#4fc3f7}
.fb-btn{background:#333;border:1px solid #555;color:#4fc3f7;padding:2px 8px;border-radius:4px;cursor:pointer;font-size:.9em}
.fb-btn:hover{background:#444}
.note-cell{max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#aaa;font-size:.8em}

/* Modal */
.modal-bg{display:none;position:fixed;top:0;left:0;width:100%%;height:100%%;background:rgba(0,0,0,.7);z-index:100;justify-content:center;align-items:center}
.modal-bg.open{display:flex}
.modal{background:#1a1d28;border:1px solid #333;border-radius:12px;padding:1.5em;width:420px;max-width:95vw}
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
</style>
</head>
<body>
<h1>SpaceGuardian Dashboard</h1>
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
  <th>เวลา</th><th>กล้อง</th><th>ตรวจพบ</th><th>Sub-Label</th><th>ความมั่นใจ</th>
  <th>โซน</th><th>ระบุตัวตน</th><th>ข้อมูลรถ</th><th>หมายเหตุ</th><th></th>
</tr>
%s
</table>
</div>

<!-- Feedback Modal -->
<div class="modal-bg" id="feedbackModal">
<div class="modal">
  <h3>ระบุข้อมูลเพิ่มเติม</h3>
  <input type="hidden" id="fb-event-id">
  <p style="color:#888;font-size:.85em">Event: <span id="fb-label" style="color:#4fc3f7"></span></p>

  <label>ระบุตัวตน (ชื่อคน / ทะเบียนรถ)</label>
  <input type="text" id="fb-identity" placeholder="เช่น สมชาย, กข-1234">

  <label>ข้อมูลยานพาหนะ (ยี่ห้อ / สี / รุ่น)</label>
  <input type="text" id="fb-vehicle" placeholder="เช่น Toyota Camry สีขาว">

  <label>หมายเหตุ / Feedback</label>
  <textarea id="fb-note" placeholder="เช่น ตรวจจับถูกต้อง, เป็นคนส่งของ"></textarea>

  <div class="actions">
    <button class="btn-cancel" onclick="closeFeedback()">ยกเลิก</button>
    <button class="btn-save" onclick="saveFeedback()">บันทึก</button>
  </div>
  <div class="msg" id="fb-msg"></div>
</div>
</div>

<div class="links">
  <a href="/api/events">API: Events</a>
  <a href="/api/status">API: Status</a>
  <a href="/healthz">Health Check</a>
</div>
<p style="margin-top:1em;color:#666;font-size:.8em">Auto-refresh ทุก 10 วินาที</p>

<script>
function openFeedback(eventId, label, identity, vehicle, note) {
  document.getElementById('fb-event-id').value = eventId;
  document.getElementById('fb-label').textContent = label + ' (' + eventId.substring(0,8) + '...)';
  document.getElementById('fb-identity').value = identity || '';
  document.getElementById('fb-vehicle').value = vehicle || '';
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
    vehicle_info: document.getElementById('fb-vehicle').value,
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
</script>
</body>
</html>`
