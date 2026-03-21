package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/coinbaseblock/squareguardian/internal/detector"
)

// Handler serves the SpaceGuardian HTTP API.
type Handler struct {
	det *detector.Detector
	mux *http.ServeMux
}

// New creates a new API handler.
func New(det *detector.Detector) *Handler {
	h := &Handler{det: det, mux: http.NewServeMux()}
	h.mux.HandleFunc("/healthz", h.healthz)
	h.mux.HandleFunc("/api/events", h.events)
	h.mux.HandleFunc("/api/status", h.status)
	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
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
