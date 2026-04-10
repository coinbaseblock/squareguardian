package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"squareguardian/internal/storage"
	"squareguardian/internal/ws"
)

// EngineAPI provides REST endpoints for the event engine.
type EngineAPI struct {
	store *storage.Store
	hub   *ws.Hub
}

// NewEngineAPI creates a new engine API handler.
func NewEngineAPI(store *storage.Store, hub *ws.Hub) *EngineAPI {
	return &EngineAPI{store: store, hub: hub}
}

// RegisterRoutes registers engine API routes on the given mux.
func (a *EngineAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/events", a.handleEvents)
	mux.HandleFunc("/api/v2/events/", a.handleEventByID)
	mux.HandleFunc("/api/v2/persons", a.handlePersons)
	mux.HandleFunc("/api/v2/vehicles", a.handleVehicles)
	mux.HandleFunc("/api/v2/stats", a.handleStats)
	mux.HandleFunc("/ws", a.hub.HandleWS)
}

func (a *EngineAPI) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	camera := r.URL.Query().Get("camera")
	label := r.URL.Query().Get("label")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	events, err := a.store.QueryEvents(camera, label, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (a *EngineAPI) handleEventByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/v2/events/"):]
	if id == "" {
		http.Error(w, "event id required", http.StatusBadRequest)
		return
	}

	event, err := a.store.GetEvent(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if event == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (a *EngineAPI) handlePersons(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		persons, err := a.store.ListPersons()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, persons)
	case http.MethodPost:
		var p storage.Person
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if p.ID == "" || p.Name == "" {
			http.Error(w, "id and name required", http.StatusBadRequest)
			return
		}
		if err := a.store.InsertPerson(&p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *EngineAPI) handleVehicles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		vehicles, err := a.store.ListVehicles()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, vehicles)
	case http.MethodPost:
		var v storage.Vehicle
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if v.ID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		if err := a.store.InsertVehicle(&v); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *EngineAPI) handleStats(w http.ResponseWriter, r *http.Request) {
	count, err := a.store.EventCount()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event_count":    count,
		"ws_connections": a.hub.Count(),
	})
}
