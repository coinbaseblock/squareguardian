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
		snapLink := ""
		if e.Snapshot != "" {
			snapLink = fmt.Sprintf(`<a href="/api/events?label=%s" target="_blank">📷</a>`, e.Label)
		}
		eventRows += fmt.Sprintf(
			`<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			ts, e.Camera, e.Label, score, zone, snapLink)
	}
	if eventRows == "" {
		eventRows = `<tr><td colspan="6" style="text-align:center;color:#888;padding:2em">ยังไม่มี event — รอ Frigate ตรวจจับ...</td></tr>`
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SpaceGuardian — Dashboard</title>
<meta http-equiv="refresh" content="5">
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
td{padding:.5em 1em;border-top:1px solid #252836}
tr:hover{background:#252836}
a{color:#4fc3f7;text-decoration:none}
.section{margin-bottom:1.5em}
.section h2{font-size:1.1em;margin-bottom:.5em;color:#ccc}
.status{display:inline-block;padding:2px 10px;border-radius:12px;font-size:.8em;font-weight:bold}
.status.ok{background:#1b5e20;color:#a5d6a7}
.status.warn{background:#e65100;color:#ffcc80}
.links{margin-top:1em;display:flex;gap:1em}
.links a{background:#252836;padding:.5em 1em;border-radius:6px;color:#4fc3f7}
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
<tr><th>เวลา</th><th>กล้อง</th><th>ตรวจพบ</th><th>ความมั่นใจ</th><th>โซน</th><th></th></tr>
%s
</table>
</div>

<div class="links">
  <a href="/api/events">API: Events</a>
  <a href="/api/status">API: Status</a>
  <a href="/healthz">Health Check</a>
</div>
<p style="margin-top:1em;color:#666;font-size:.8em">Auto-refresh ทุก 5 วินาที</p>
</body>
</html>`, len(events), len(labels), labelRows, eventRows)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: json encode error: %v", err)
	}
}
