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
	"strconv"
	"strings"
	"time"

	"squareguardian/internal/detector"
)

// Handler serves the SquareGuardian HTTP API.
type Handler struct {
	det            *detector.Detector
	cameraZones    map[string]string
	faceServiceURL string
	timezone       *time.Location
	timezoneName   string
	mux            *http.ServeMux
}

// New creates a new API handler.
func New(det *detector.Detector, cameraZones map[string]string, faceServiceURL string, timezone string) *Handler {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("api: invalid timezone %q, falling back to Asia/Bangkok", timezone)
		loc, _ = time.LoadLocation("Asia/Bangkok")
		timezone = "Asia/Bangkok"
	}
	h := &Handler{
		det:            det,
		cameraZones:    make(map[string]string, len(cameraZones)),
		faceServiceURL: faceServiceURL,
		timezone:       loc,
		timezoneName:   timezone,
		mux:            http.NewServeMux(),
	}
	for camera, zone := range cameraZones {
		h.cameraZones[strings.TrimSpace(camera)] = strings.TrimSpace(zone)
	}
	h.mux.HandleFunc("/", h.dashboard)
	h.mux.HandleFunc("/events", h.eventsPage)
	h.mux.HandleFunc("/faces", h.facesPage)
	h.mux.HandleFunc("/healthz", h.healthz)
	h.mux.HandleFunc("/api/events", h.events)
	h.mux.HandleFunc("/api/status", h.status)
	h.mux.HandleFunc("/api/annotate", h.annotate)
	h.mux.HandleFunc("/api/thumbnail/", h.thumbnail)
	h.mux.HandleFunc("/api/snapshot/", h.snapshot)
	h.mux.HandleFunc("/api/group", h.group)
	h.mux.HandleFunc("/api/groups", h.groups)
	h.mux.HandleFunc("/api/groups/delete", h.deleteGroup)
	h.mux.HandleFunc("/api/training-data", h.trainingData)
	h.mux.HandleFunc("/api/events/identified", h.identifiedEvents)
	h.mux.HandleFunc("/api/cameras", h.cameras)
	h.mux.HandleFunc("/api/camera-snapshot/", h.cameraSnapshot)
	h.mux.HandleFunc("/api/face/", h.faceProxy)
	h.mux.HandleFunc("/weights", h.weightsPage)
	h.mux.HandleFunc("/api/weights", h.weightsAPI)
	h.mux.HandleFunc("/api/suggest-identity", h.suggestIdentity)
	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// commonTimezones is the list of timezone options available in the UI.
var commonTimezones = []string{
	"Asia/Bangkok",
	"Asia/Tokyo",
	"Asia/Shanghai",
	"Asia/Singapore",
	"Asia/Kolkata",
	"Asia/Dubai",
	"Europe/London",
	"Europe/Paris",
	"Europe/Berlin",
	"America/New_York",
	"America/Chicago",
	"America/Denver",
	"America/Los_Angeles",
	"Pacific/Auckland",
	"Australia/Sydney",
	"UTC",
}

// resolveTimezone returns the timezone location to use for this request.
// Priority: ?tz= query param > server default.
func (h *Handler) resolveTimezone(r *http.Request) *time.Location {
	if tz := r.URL.Query().Get("tz"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return h.timezone
}

func (h *Handler) buildTimezoneOptions(selected string) string {
	opts := ""
	for _, tz := range commonTimezones {
		sel := ""
		if tz == selected {
			sel = " selected"
		}
		opts += fmt.Sprintf(`<option value="%s"%s>%s</option>`, tz, sel, tz)
	}
	return opts
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	loc := h.resolveTimezone(r)
	cameraFilter := r.URL.Query().Get("camera")
	identityFilter := r.URL.Query().Get("identity")
	events := h.det.EventsFiltered("", cameraFilter)
	labels := h.det.TrackedLabels()
	cameras := h.det.Cameras()
	personIdentities := h.det.PersonIdentities()

	// Apply identity filter (person events only)
	if identityFilter != "" {
		var filtered []detector.Event
		for _, e := range events {
			if e.Label != "person" {
				continue
			}
			ident := e.Identity
			if ident == "" {
				ident = "_unknown"
			}
			if ident == identityFilter {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}

	// Count events by label
	counts := make(map[string]int)
	for _, e := range events {
		counts[e.Label]++
	}

	// Build camera filter buttons
	baseFilterParams := ""
	if identityFilter != "" {
		baseFilterParams += "&identity=" + identityFilter
	}
	cameraButtons := fmt.Sprintf(`<a href="/?%s" class="cam-btn%s" data-i18n="all_cameras">All</a>`, strings.TrimPrefix(baseFilterParams, "&"), boolClass(cameraFilter == "", " active"))
	for _, cam := range cameras {
		cameraButtons += fmt.Sprintf(`<a href="/?camera=%s%s" class="cam-btn%s">%s</a>`,
			cam, baseFilterParams, boolClass(cameraFilter == cam, " active"), cam)
	}

	// Build person identity filter buttons
	identityFilterParams := ""
	if cameraFilter != "" {
		identityFilterParams = "&camera=" + cameraFilter
	}
	identityButtons := fmt.Sprintf(`<a href="/?%s" class="cam-btn%s" data-i18n="all_identities">All</a>`, strings.TrimPrefix(identityFilterParams, "&"), boolClass(identityFilter == "", " active"))
	for _, ident := range personIdentities {
		displayName := ident
		if ident == "_unknown" {
			displayName = "Unknown"
		}
		identityButtons += fmt.Sprintf(`<a href="/?identity=%s%s" class="cam-btn%s">%s</a>`,
			ident, identityFilterParams, boolClass(identityFilter == ident, " active"), displayName)
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

	// Temporarily set handler timezone for row rendering
	origLoc := h.timezone
	h.timezone = loc
	defer func() { h.timezone = origLoc }()

	// Pagination for recent events
	perPage := 20
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if pv, err := strconv.Atoi(p); err == nil && pv > 0 {
			page = pv
		}
	}
	totalEvents := len(events)
	totalPages := (totalEvents + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * perPage
	end := start + perPage
	if end > totalEvents {
		end = totalEvents
	}

	eventRows := ""
	if totalEvents > 0 {
		for _, e := range events[start:end] {
			eventRows += h.buildEventRow(e)
		}
	}
	if eventRows == "" {
		eventRows = `<tr><td colspan="14" style="text-align:center;color:#888;padding:2em" data-i18n="no_events">No events yet — waiting for Frigate to detect...</td></tr>`
	}

	// Build pagination controls
	paginationHTML := ""
	if totalPages > 1 {
		// Build base URL preserving existing query params
		baseParams := ""
		if cameraFilter != "" {
			baseParams += "&camera=" + cameraFilter
		}
		if identityFilter != "" {
			baseParams += "&identity=" + identityFilter
		}
		if tz := r.URL.Query().Get("tz"); tz != "" {
			baseParams += "&tz=" + tz
		}

		paginationHTML = `<div style="display:flex;justify-content:center;align-items:center;gap:12px;margin-top:16px;flex-wrap:wrap">`
		if page > 1 {
			paginationHTML += fmt.Sprintf(`<a href="/?page=%d%s" class="btn btn-secondary btn-sm">&laquo; Prev</a>`, page-1, baseParams)
		}
		// Show page numbers with ellipsis
		for i := 1; i <= totalPages; i++ {
			if i == 1 || i == totalPages || (i >= page-2 && i <= page+2) {
				if i == page {
					paginationHTML += fmt.Sprintf(`<span style="background:#3b82f6;color:#fff;padding:4px 10px;border-radius:4px;font-weight:bold">%d</span>`, i)
				} else {
					paginationHTML += fmt.Sprintf(`<a href="/?page=%d%s" style="padding:4px 10px;color:#93c5fd;text-decoration:none">%d</a>`, i, baseParams, i)
				}
			} else if i == page-3 || i == page+3 {
				paginationHTML += `<span style="color:#6b7280">...</span>`
			}
		}
		if page < totalPages {
			paginationHTML += fmt.Sprintf(`<a href="/?page=%d%s" class="btn btn-secondary btn-sm">Next &raquo;</a>`, page+1, baseParams)
		}
		paginationHTML += `</div>`
	}

	// Recent events heading with range info
	eventsHeading := fmt.Sprintf("Recent Events (%d-%d of %d)", start+1, end, totalEvents)
	if totalEvents == 0 {
		eventsHeading = "Recent Events"
	}

	tzOpts := h.buildTimezoneOptions(loc.String())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, dashboardTpl, tzOpts, len(events), len(labels), cameraButtons, identityButtons, labelRows, eventsHeading, eventRows, paginationHTML, h.timezoneName)
}

func boolClass(cond bool, class string) string {
	if cond {
		return class
	}
	return ""
}

func (h *Handler) eventsPage(w http.ResponseWriter, r *http.Request) {
	loc := h.resolveTimezone(r)
	origLoc := h.timezone
	h.timezone = loc
	defer func() { h.timezone = origLoc }()

	events := h.det.Events("")
	labels := h.det.TrackedLabels()
	groups := h.det.Groups()

	// Sort labels for consistent display
	sort.Strings(labels)

	// Group events by label
	grouped := make(map[string][]detector.Event)
	for _, e := range events {
		grouped[e.Label] = append(grouped[e.Label], e)
	}

	// For person events, sub-group by identity (known persons first, then cluster unknowns).
	// Unknown persons are clustered by camera + time proximity (within 5 min window).
	type personCluster struct {
		Name   string
		Events []detector.Event
	}
	var personClusters []personCluster
	if personEvents, ok := grouped["person"]; ok && len(personEvents) > 0 {
		// 1. Group identified persons
		identMap := make(map[string][]detector.Event)
		var unknowns []detector.Event
		for _, e := range personEvents {
			if e.Identity != "" {
				identMap[e.Identity] = append(identMap[e.Identity], e)
			} else {
				unknowns = append(unknowns, e)
			}
		}
		// Sort identities alphabetically, but put user-annotated (non-"outsider") first
		var identNames []string
		for name := range identMap {
			identNames = append(identNames, name)
		}
		sort.Slice(identNames, func(i, j int) bool {
			ai := identNames[i] == "outsider"
			aj := identNames[j] == "outsider"
			if ai != aj {
				return !ai // non-outsider first
			}
			return identNames[i] < identNames[j]
		})
		for _, name := range identNames {
			personClusters = append(personClusters, personCluster{Name: name, Events: identMap[name]})
		}
		// 2. Cluster unknown persons by camera + time proximity (5 min)
		type cameraTimeKey struct {
			camera string
			bucket int64
		}
		clusterMap := make(map[cameraTimeKey][]detector.Event)
		var clusterKeys []cameraTimeKey
		for _, e := range unknowns {
			bucket := int64(e.StartTime) / 300 // 5 min buckets
			key := cameraTimeKey{camera: e.Camera, bucket: bucket}
			if _, exists := clusterMap[key]; !exists {
				clusterKeys = append(clusterKeys, key)
			}
			clusterMap[key] = append(clusterMap[key], e)
		}
		// Sort clusters by most recent first
		sort.Slice(clusterKeys, func(i, j int) bool {
			return clusterKeys[i].bucket > clusterKeys[j].bucket
		})
		for idx, key := range clusterKeys {
			name := fmt.Sprintf("Unknown #%d (%s)", idx+1, key.camera)
			personClusters = append(personClusters, personCluster{Name: name, Events: clusterMap[key]})
		}
	}

	// Build sections for each label
	sections := ""
	// Helper to build thumbnail cards for a slice of events
	buildThumbs := func(evts []detector.Event, showLimit int) (string, string) {
		if len(evts) < showLimit {
			showLimit = len(evts)
		}
		thumbs := ""
		for _, e := range evts[:showLimit] {
			t := time.Unix(int64(e.StartTime), int64(math.Mod(e.StartTime, 1)*1e9)).In(h.timezone)
			ts := t.Format("15:04")
			ago := timeAgo(e.StartTime)

			identBadge := ""
			if e.Identity != "" {
				identBadge = fmt.Sprintf(`<span class="ident-badge">%s</span>`, e.Identity)
			}
			if e.RoomNumber != "" {
				identBadge += fmt.Sprintf(`<span class="room-badge">%s</span>`, e.RoomNumber)
			}

			groupBadge := ""
			if e.GroupID != "" {
				gname := ""
				for _, g := range groups {
					if g.ID == e.GroupID {
						gname = g.Name
						break
					}
				}
				if gname != "" {
					groupBadge = fmt.Sprintf(`<span class="group-badge">%s</span>`, gname)
				}
			}

			var thumbSrc string
			if e.Thumbnail != "" {
				thumbSrc = fmt.Sprintf("data:image/jpeg;base64,%s", e.Thumbnail)
			} else {
				thumbSrc = fmt.Sprintf("/api/thumbnail/%s", e.ID)
			}

			thumbs += fmt.Sprintf(`<div class="ev-card" data-event-id="%s" data-label="%s" onclick="handleCardClick(event, '%s','%s','%s','%s','%s','%s','%s','%s','%s')">
				<input type="checkbox" class="ev-checkbox" data-event-id="%s" onclick="event.stopPropagation(); toggleSelect('%s')">
				<img src="%s" alt="%s" loading="lazy">
				<div class="ev-time">%s · %s</div>
				%s%s
			</div>`,
				e.ID, e.Label,
				e.ID, e.Label,
				escapeJS(e.Identity), escapeJS(e.RoomNumber),
				escapeJS(e.LicensePlate), escapeJS(e.Province),
				escapeJS(e.VehicleBrand), escapeJS(e.VehicleColor),
				escapeJS(e.Note),
				e.ID, e.ID,
				thumbSrc, e.Label,
				ts, ago,
				identBadge, groupBadge)
		}
		moreIndicator := ""
		if len(evts) > showLimit {
			moreIndicator = fmt.Sprintf(`<div class="more-indicator">+%d more</div>`, len(evts)-showLimit)
		}
		return thumbs, moreIndicator
	}

	for _, label := range labels {
		evts := grouped[label]
		if len(evts) == 0 {
			continue
		}

		displayName := labelDisplayName(label)

		// For person label: render sub-sections grouped by identity
		if label == "person" && len(personClusters) > 0 {
			sections += fmt.Sprintf(`<div class="ev-section">
				<h2>%s <span class="ev-count">%d Tracked Objects</span></h2>`, displayName, len(evts))
			for _, cluster := range personClusters {
				thumbs, moreIndicator := buildThumbs(cluster.Events, 30)
				clusterLabel := cluster.Name
				sections += fmt.Sprintf(`<div style="margin:0.8em 0 0.3em 0">
					<h3 style="font-size:0.95em;color:#4fc3f7;margin-bottom:0.3em">%s <span style="color:#888;font-size:0.85em">(%d)</span></h3>
					<div class="ev-grid">%s%s</div>
				</div>`, clusterLabel, len(cluster.Events), thumbs, moreIndicator)
			}
			sections += `</div>`
			continue
		}

		thumbs, moreIndicator := buildThumbs(evts, 50)

		sections += fmt.Sprintf(`<div class="ev-section">
			<h2>%s <span class="ev-count">%d Tracked Objects</span></h2>
			<div class="ev-grid">%s%s</div>
		</div>`, displayName, len(evts), thumbs, moreIndicator)
	}

	if sections == "" {
		sections = `<div style="text-align:center;color:#888;padding:3em">No events yet — waiting for Frigate to detect...</div>`
	}

	// Build existing groups summary
	groupsSummary := ""
	if len(groups) > 0 {
		groupsSummary = `<div class="ev-section" style="border-top:1px solid #333;padding-top:1em;margin-top:1em">
			<h2>Created Groups <span class="ev-count">` + fmt.Sprintf("%d", len(groups)) + ` Groups</span></h2>`
		for _, g := range groups {
			// Find event thumbnails for this group
			thumbPreviews := ""
			count := 0
			for _, e := range events {
				if e.GroupID == g.ID && count < 5 {
					var src string
					if e.Thumbnail != "" {
						src = fmt.Sprintf("data:image/jpeg;base64,%s", e.Thumbnail)
					} else {
						src = fmt.Sprintf("/api/thumbnail/%s", e.ID)
					}
					thumbPreviews += fmt.Sprintf(`<img src="%s" style="width:50px;height:40px;object-fit:cover;border-radius:4px">`, src)
					count++
				}
			}
			groupsSummary += fmt.Sprintf(`<div class="group-row">
				<div class="group-info">
					<strong>%s</strong> <span style="color:#888">(%s · %d images)</span>
				</div>
				<div class="group-thumbs">%s</div>
				<button class="btn-delete-group" onclick="deleteGroup('%s')">Delete Group</button>
			</div>`, g.Name, labelDisplayName(g.Label), len(g.EventIDs), thumbPreviews, g.ID)
		}
		groupsSummary += `</div>`
	}

	tzOpts := h.buildTimezoneOptions(loc.String())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, eventsPageTpl, tzOpts, sections, groupsSummary, h.timezoneName)
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	label := r.URL.Query().Get("label")
	camera := r.URL.Query().Get("camera")
	events := h.det.EventsFiltered(label, camera)
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(events),
		"events": events,
	})
}

func (h *Handler) identifiedEvents(w http.ResponseWriter, _ *http.Request) {
	identified := h.det.IdentifiedEvents(0) // no limit — return all events per person
	writeJSON(w, http.StatusOK, identified)
}

func (h *Handler) cameras(w http.ResponseWriter, _ *http.Request) {
	cameras := h.det.Cameras()
	writeJSON(w, http.StatusOK, map[string]any{
		"cameras": cameras,
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

// cameraSnapshot returns the latest frame from a camera via Frigate.
func (h *Handler) cameraSnapshot(w http.ResponseWriter, r *http.Request) {
	camera := strings.TrimPrefix(r.URL.Path, "/api/camera-snapshot/")
	if camera == "" {
		http.Error(w, "camera name required", http.StatusBadRequest)
		return
	}
	h.proxyFrigate(w, fmt.Sprintf("/api/%s/latest.jpg?h=720", camera))
}

func (h *Handler) group(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name     string   `json:"name"`
		Label    string   `json:"label"`
		EventIDs []string `json:"event_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Name == "" || len(req.EventIDs) < 2 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required and at least 2 event_ids"})
		return
	}
	groupID := h.det.GroupEvents(req.Name, req.Label, req.EventIDs)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "group_id": groupID})
}

func (h *Handler) groups(w http.ResponseWriter, r *http.Request) {
	groups := h.det.Groups()
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(groups),
		"groups": groups,
	})
}

func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		GroupID string `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if h.det.DeleteGroup(req.GroupID) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	} else {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
	}
}

func (h *Handler) trainingData(w http.ResponseWriter, r *http.Request) {
	groups := h.det.Groups()
	events := h.det.Events("")

	type TrainingEntry struct {
		GroupID   string           `json:"group_id"`
		GroupName string           `json:"group_name"`
		Label     string           `json:"label"`
		Events    []detector.Event `json:"events"`
	}

	eventMap := make(map[string]detector.Event)
	for _, e := range events {
		eventMap[e.ID] = e
	}

	var entries []TrainingEntry
	for _, g := range groups {
		var gevts []detector.Event
		for _, eid := range g.EventIDs {
			if e, ok := eventMap[eid]; ok {
				gevts = append(gevts, e)
			}
		}
		entries = append(entries, TrainingEntry{
			GroupID:   g.ID,
			GroupName: g.Name,
			Label:     g.Label,
			Events:    gevts,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count":         len(entries),
		"training_data": entries,
	})
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

func (h *Handler) weightsPage(w http.ResponseWriter, r *http.Request) {
	loc := h.resolveTimezone(r)
	origLoc := h.timezone
	h.timezone = loc
	defer func() { h.timezone = origLoc }()

	// Get unidentified person events (max 50 for the page)
	unidentified := h.det.UnidentifiedPersonEvents(50)

	// Get known identities with representative thumbnails
	knownIdents := h.det.KnownIdentities()

	// Build known identity cards HTML
	knownHTML := ""
	if len(knownIdents) == 0 {
		knownHTML = `<p style="color:#888;font-size:.9em" data-i18n="no_known_identities">No identified persons yet. Use the feedback button on the Dashboard to manually identify someone first.</p>`
	} else {
		knownHTML = `<div class="ident-grid">`
		for name, e := range knownIdents {
			weightLabel := "manual"
			if e.IdentityWeight > 0 && e.IdentityWeight < 1.0 {
				weightLabel = fmt.Sprintf("suggested %.0f%%", e.IdentityWeight*100)
			}
			thumbSrc := fmt.Sprintf("/api/thumbnail/%s", e.ID)
			if e.Thumbnail != "" {
				thumbSrc = fmt.Sprintf("data:image/jpeg;base64,%s", e.Thumbnail)
			}
			knownHTML += fmt.Sprintf(
				`<div class="ident-card" data-identity="%s">
					<img src="%s" alt="%s">
					<div class="ident-name">%s</div>
					<div class="ident-weight">%s</div>
				</div>`,
				escapeJS(name), thumbSrc, escapeJS(name), name, weightLabel)
		}
		knownHTML += `</div>`
	}

	// Build unidentified event cards
	unidentHTML := ""
	if len(unidentified) == 0 {
		unidentHTML = `<p style="color:#888;font-size:.9em" data-i18n="no_unidentified">All person events have been identified!</p>`
	} else {
		unidentHTML = `<div class="unident-grid">`
		for _, e := range unidentified {
			t := time.Unix(int64(e.StartTime), 0).In(h.timezone)
			ts := t.Format("2006-01-02 15:04:05")
			thumbSrc := fmt.Sprintf("/api/thumbnail/%s", e.ID)
			if e.Thumbnail != "" {
				thumbSrc = fmt.Sprintf("data:image/jpeg;base64,%s", e.Thumbnail)
			}
			zone := h.displayZone(e)
			if zone == "" {
				zone = e.Camera
			}

			// Build identity option buttons
			identBtns := ""
			for name := range knownIdents {
				identBtns += fmt.Sprintf(
					`<button class="suggest-btn" onclick="suggestIdent('%s','%s',0.5)">%s</button>`,
					e.ID, escapeJS(name), name)
			}
			identBtns += fmt.Sprintf(
				`<button class="suggest-btn new-btn" onclick="addNewIdent('%s')">+ New</button>`, e.ID)

			unidentHTML += fmt.Sprintf(
				`<div class="unident-card" data-event-id="%s">
					<img src="%s" alt="person" class="unident-thumb">
					<div class="unident-info">
						<div class="unident-time">%s</div>
						<div class="unident-camera">%s · %s</div>
					</div>
					<div class="unident-actions">
						<div class="suggest-label" data-i18n="who_is_this">Who is this?</div>
						<div class="suggest-btns">%s</div>
						<div class="suggest-weight">
							<label data-i18n="confidence_level">Confidence:</label>
							<input type="range" min="10" max="90" value="50" class="weight-slider" id="weight-%s"
								oninput="document.getElementById('wv-%s').textContent=this.value+'%%'">
							<span id="wv-%s" class="weight-val">50%%</span>
						</div>
					</div>
				</div>`,
				e.ID, thumbSrc, ts, e.Camera, zone, identBtns, e.ID, e.ID, e.ID)
		}
		unidentHTML += `</div>`
	}

	// Label weights section
	labels := h.det.TrackedLabels()
	sort.Strings(labels)
	weights := h.det.GetWeights()
	weightMap := make(map[string]float64)
	for _, w := range weights {
		weightMap[w.Label] = w.Weight
	}
	labelWeightRows := ""
	for _, l := range labels {
		w := weightMap[l]
		pct := int(w * 100)
		labelWeightRows += fmt.Sprintf(
			`<tr>
				<td>%s</td>
				<td>
					<input type="range" min="0" max="49" value="%d" class="label-weight-slider"
						id="lw-%s" oninput="document.getElementById('lwv-%s').textContent=this.value+'%%'">
					<span id="lwv-%s" class="weight-val">%d%%</span>
				</td>
				<td><button class="btn-save-weight" onclick="saveLabelWeight('%s',document.getElementById('lw-%s').value)" data-i18n="save">Save</button></td>
			</tr>`,
			labelDisplayName(l), pct, l, l, l, pct, l, l)
	}

	tzOpts := h.buildTimezoneOptions(loc.String())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, weightsPageTpl,
		tzOpts,
		knownHTML,
		len(unidentified),
		unidentHTML,
		labelWeightRows,
		h.timezoneName)
}

func (h *Handler) weightsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		weights := h.det.GetWeights()
		writeJSON(w, http.StatusOK, map[string]any{"weights": weights})
	case http.MethodPost:
		var req struct {
			Label  string  `json:"label"`
			Weight float64 `json:"weight"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Label == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "label required"})
			return
		}
		h.det.SetWeight(req.Label, req.Weight/100) // input is percentage
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) suggestIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		EventID  string  `json:"event_id"`
		Identity string  `json:"identity"`
		Weight   float64 `json:"weight"` // 0-100 percentage
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.EventID == "" || req.Identity == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event_id and identity required"})
		return
	}
	weight := req.Weight / 100 // convert percentage to 0-1 range
	if h.det.SuggestIdentity(req.EventID, req.Identity, weight) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	} else {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
	}
}

func (h *Handler) buildEventRow(e detector.Event) string {
	t := time.Unix(int64(e.StartTime), int64(math.Mod(e.StartTime, 1)*1e9)).In(h.timezone)
	ts := t.Format("2006-01-02 15:04:05")
	effectiveScore := h.det.EffectiveScore(e)
	score := fmt.Sprintf("%.0f%%", effectiveScore*100)
	if e.TopScore == 0 && effectiveScore > 0 {
		score += " ⚖" // weight-based indicator
	}
	zone := h.displayZone(e)
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
	} else if e.IdentityWeight > 0 && e.IdentityWeight < 1.0 {
		identityDisplay += fmt.Sprintf(` <span style="color:#ff9800;font-size:.75em">(~%.0f%%)</span>`, e.IdentityWeight*100)
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

func (h *Handler) displayZone(e detector.Event) string {
	zone := strings.TrimSpace(e.Zone)
	if zone != "" {
		return zone
	}
	camera := strings.TrimSpace(e.Camera)
	if h.cameraZones != nil {
		if mappedZone := strings.TrimSpace(h.cameraZones[camera]); mappedZone != "" {
			return mappedZone
		}
	}
	return camera
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
		"person":     "Person",
		"car":        "Car",
		"motorcycle": "Motorcycle",
		"bus":        "Bus",
		"truck":      "Truck",
		"backpack":   "Backpack",
		"suitcase":   "Suitcase",
		"handbag":    "Handbag",
	}
	if n, ok := names[label]; ok {
		return n
	}
	return label
}

// i18nScript provides client-side bilingual (Thai/English) translation and timezone selection.
const i18nScript = `
<script>
var i18n = {
  th: {
    dashboard: "Dashboard",
    events_by_type: "Events (แยกประเภท)",
    face_gallery: "Face Gallery",
    all_events: "Events ทั้งหมด",
    tracked_types: "ประเภทที่ติดตาม",
    system_status: "สถานะระบบ",
    filter_by_camera: "กรองตามกล้อง",
    all_cameras: "ทั้งหมด",
    summary_by_type: "สรุปตามประเภท",
    type_col: "ประเภท",
    event_count: "จำนวน Event",
    recent_events: "Events ล่าสุด",
    image: "ภาพ",
    time: "เวลา",
    camera: "กล้อง",
    detected: "ตรวจพบ",
    confidence: "ความมั่นใจ",
    zone: "โซน",
    identity: "ระบุตัวตน",
    room: "ห้อง",
    license_plate: "ทะเบียน",
    brand_color: "ยี่ห้อ/สี",
    notes: "หมายเหตุ",
    no_events: "ยังไม่มี event — รอ Frigate ตรวจจับ...",
    auto_refresh: "Auto-refresh ทุก 10 วินาที",
    select_multiple: "เลือกหลายรายการ",
    click_to_identify: "คลิกที่รูปเพื่อระบุตัวตน, ห้อง, ทะเบียนรถ, ยี่ห้อ/สี",
    selected_count: "เลือกแล้ว",
    items: "รายการ",
    create_group: "สร้างกลุ่ม",
    cancel: "ยกเลิก",
    new_group: "สร้างกลุ่มใหม่",
    group_desc: "รวมภาพที่เลือกเป็นกลุ่มเดียวกัน (เช่น คนเดียวกัน, รถคันเดียวกัน)",
    group_name: "ชื่อกลุ่ม",
    type_label: "ประเภท",
    selected_images: "ภาพที่เลือก",
    existing_groups: "กลุ่มที่สร้างแล้ว",
    delete_group: "ลบกลุ่ม",
    save: "บันทึก",
    additional_info: "ระบุข้อมูลเพิ่มเติม",
    identity_name: "ระบุตัวตน (ชื่อคน)",
    room_unit: "เลขห้อง / Unit",
    license: "ทะเบียนรถ",
    province: "จังหวัด (ถ้ามี)",
    brand_model: "ยี่ห้อ / รุ่น",
    vehicle_color: "สีรถ",
    feedback: "หมายเหตุ / Feedback",
    saved_success: "บันทึกสำเร็จ!",
    group_created: "สร้างกลุ่มสำเร็จ!",
    enter_group_name: "กรุณาใส่ชื่อกลุ่ม",
    select_at_least_2: "กรุณาเลือกอย่างน้อย 2 รายการ",
    confirm_delete: "ต้องการลบกลุ่มนี้?",
    just_now: "เมื่อสักครู่",
    more_items: "รายการ",
    timezone: "เขตเวลา",
    language: "ภาษา",
    images: "ภาพ",
    face_status_online: "พร้อมใช้งาน",
    face_status_offline: "ไม่ได้เชื่อมต่อ",
    outside_person: "คนภายนอก",
    filter_by_person: "กรองตามบุคคล",
    all_identities: "ทั้งหมด",
    weight_detection: "ตรวจจับน้ำหนัก",
    weight_detection_title: "SquareGuardian — ตรวจจับน้ำหนัก",
    weight_page_desc: "เสนอตัวตนสำหรับบุคคลที่ยังไม่ระบุ ตัวตนที่เสนอมีน้ำหนักต่ำกว่าการระบุด้วยตนเองจาก Dashboard",
    known_identities: "บุคคลที่รู้จัก",
    unidentified_persons: "บุคคลที่ยังไม่ระบุตัวตน",
    label_weights: "น้ำหนักการตรวจจับตามประเภท",
    label_weight_desc: "ตั้งค่าน้ำหนักเริ่มต้นสำหรับแต่ละประเภท น้ำหนักเหล่านี้ต่ำกว่าคะแนนจาก Frigate เสมอ (สูงสุด 49%)",
    who_is_this: "คนนี้คือใคร?",
    confidence_level: "ระดับความมั่นใจ:",
    add_new_identity: "เพิ่มตัวตนใหม่",
    no_known_identities: "ยังไม่มีบุคคลที่ระบุตัวตน ใช้ปุ่มแก้ไขบน Dashboard เพื่อระบุตัวตนด้วยตนเองก่อน",
    no_unidentified: "ระบุตัวตนบุคคลครบทุกคนแล้ว!",
    weight: "น้ำหนัก"
  },
  en: {
    dashboard: "Dashboard",
    events_by_type: "Events (by type)",
    face_gallery: "Face Gallery",
    all_events: "Total Events",
    tracked_types: "Tracked Types",
    system_status: "System Status",
    filter_by_camera: "Filter by Camera",
    all_cameras: "All",
    summary_by_type: "Summary by Type",
    type_col: "Type",
    event_count: "Event Count",
    recent_events: "Recent Events",
    image: "Image",
    time: "Time",
    camera: "Camera",
    detected: "Detected",
    confidence: "Confidence",
    zone: "Zone",
    identity: "Identity",
    room: "Room",
    license_plate: "License Plate",
    brand_color: "Brand/Color",
    notes: "Notes",
    no_events: "No events yet — waiting for Frigate detection...",
    auto_refresh: "Auto-refresh every 10 seconds",
    select_multiple: "Select Multiple",
    click_to_identify: "Click image to identify, add room, license plate, brand/color",
    selected_count: "Selected",
    items: "items",
    create_group: "Create Group",
    cancel: "Cancel",
    new_group: "Create New Group",
    group_desc: "Group selected images together (e.g. same person, same vehicle)",
    group_name: "Group Name",
    type_label: "Type",
    selected_images: "Selected Images",
    existing_groups: "Existing Groups",
    delete_group: "Delete Group",
    save: "Save",
    additional_info: "Additional Information",
    identity_name: "Identity (Name)",
    room_unit: "Room / Unit",
    license: "License Plate",
    province: "Province",
    brand_model: "Brand / Model",
    vehicle_color: "Vehicle Color",
    feedback: "Notes / Feedback",
    saved_success: "Saved successfully!",
    group_created: "Group created!",
    enter_group_name: "Please enter group name",
    select_at_least_2: "Please select at least 2 items",
    confirm_delete: "Delete this group?",
    just_now: "just now",
    more_items: "items",
    timezone: "Timezone",
    language: "Language",
    images: "images",
    face_status_online: "Online",
    face_status_offline: "Not connected",
    outside_person: "Unknown person",
    filter_by_person: "Filter by Person",
    all_identities: "All",
    weight_detection: "Weight Detection",
    weight_detection_title: "SquareGuardian — Weight Detection",
    weight_page_desc: "Suggest identities for unidentified persons. Suggested identities have lower confidence than manual annotations from the Dashboard.",
    known_identities: "Known Identities",
    unidentified_persons: "Unidentified Persons",
    label_weights: "Detection Weights by Type",
    label_weight_desc: "Set default weights for each detection type. These are always lower than Frigate's actual detection score (max 49%).",
    who_is_this: "Who is this?",
    confidence_level: "Confidence:",
    add_new_identity: "Add New Identity",
    no_known_identities: "No identified persons yet. Use the feedback button on the Dashboard to manually identify someone first.",
    no_unidentified: "All person events have been identified!",
    weight: "Weight"
  }
};

function getLang() {
  return localStorage.getItem('sg_lang') || 'en';
}

function setLang(lang) {
  localStorage.setItem('sg_lang', lang);
  applyLang();
}

function applyLang() {
  var lang = getLang();
  var t = i18n[lang] || i18n.en;
  document.querySelectorAll('[data-i18n]').forEach(function(el) {
    var key = el.getAttribute('data-i18n');
    if (t[key]) el.textContent = t[key];
  });
  // Update lang toggle button
  var btn = document.getElementById('langToggle');
  if (btn) btn.textContent = lang === 'en' ? 'TH' : 'EN';
}

function toggleLang() {
  setLang(getLang() === 'en' ? 'th' : 'en');
}

document.addEventListener('DOMContentLoaded', applyLang);
</script>
`

func timeAgo(startTime float64) string {
	d := time.Since(time.Unix(int64(startTime), 0))
	switch {
	case d < time.Minute:
		return "just now"
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
  <h3>Additional Information</h3>
  <input type="hidden" id="fb-event-id">
  <p style="color:#888;font-size:.85em">Event: <span id="fb-label" style="color:#4fc3f7"></span></p>

  <div class="form-row">
    <div class="form-col">
      <label>Identity (Person Name)</label>
      <input type="text" id="fb-identity" placeholder="e.g. John, Delivery Person">
    </div>
    <div class="form-col">
      <label>Room / Unit</label>
      <input type="text" id="fb-room" placeholder="e.g. A0213, B1502">
    </div>
  </div>

  <div class="form-row">
    <div class="form-col">
      <label>License Plate</label>
      <input type="text" id="fb-plate" placeholder="e.g. ABC 1234">
    </div>
    <div class="form-col">
      <label>Province (if applicable)</label>
      <input type="text" id="fb-province" placeholder="e.g. Bangkok">
    </div>
  </div>

  <div class="form-row">
    <div class="form-col">
      <label>Brand / Model</label>
      <input type="text" id="fb-brand" placeholder="e.g. Toyota Camry, Honda Civic">
    </div>
    <div class="form-col">
      <label>Vehicle Color</label>
      <input type="text" id="fb-color" placeholder="e.g. White, Black, Red">
    </div>
  </div>

  <label>Notes / Feedback</label>
  <textarea id="fb-note" placeholder="e.g. Correct detection, delivery person, vehicle from unit A0213"></textarea>

  <div class="actions">
    <button class="btn-cancel" onclick="closeFeedback()">Cancel</button>
    <button class="btn-save" onclick="saveFeedback()">Save</button>
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
      document.getElementById('fb-msg').textContent = 'Saved successfully!';
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
.cam-filter{display:flex;gap:.5em;flex-wrap:wrap;margin-bottom:1em}
.cam-btn{background:#252836;padding:.3em .8em;border-radius:6px;color:#999;font-size:.8em;text-decoration:none}
.cam-btn.active{background:#1976d2;color:#fff}
.cam-btn:hover{background:#333}
`

var dashboardTpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SquareGuardian — Dashboard</title>
<meta http-equiv="refresh" content="10">
<style>` + sharedStyles + `
.top-bar{display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:.5em;margin-bottom:.5em}
.top-controls{display:flex;gap:.5em;align-items:center}
.lang-btn{background:#252836;border:1px solid #555;color:#4fc3f7;padding:4px 12px;border-radius:6px;cursor:pointer;font-size:.8em;font-weight:bold}
.lang-btn:hover{background:#333}
.tz-select{background:#252836;border:1px solid #555;color:#4fc3f7;padding:4px 8px;border-radius:6px;font-size:.8em}
</style>
</head>
<body>
<div class="top-bar">
  <h1>SquareGuardian Dashboard</h1>
  <div class="top-controls">
    <span style="color:#888;font-size:.8em" data-i18n="timezone">Timezone</span>
    <select class="tz-select" id="tzSelect" onchange="setTimezone(this.value)">%s</select>
    <button class="lang-btn" id="langToggle" onclick="toggleLang()">TH</button>
  </div>
</div>
<div class="nav">
  <a href="/" class="active" data-i18n="dashboard">Dashboard</a>
  <a href="/events" data-i18n="events_by_type">Events by Type</a>
  <a href="/faces" data-i18n="face_gallery">Face Gallery</a>
  <a href="/weights" data-i18n="weight_detection">Weight Detection</a>
</div>

<div class="cards">
  <div class="card"><div class="num">%d</div><div class="lbl" data-i18n="all_events">All Events</div></div>
  <div class="card"><div class="num">%d</div><div class="lbl" data-i18n="tracked_types">Tracked Types</div></div>
  <div class="card"><div class="num"><span class="status ok">ONLINE</span></div><div class="lbl" data-i18n="system_status">System Status</div></div>
</div>

<div class="section">
<h2 data-i18n="filter_by_camera">Filter by Camera</h2>
<div class="cam-filter">%s</div>
</div>

<div class="section">
<h2 data-i18n="filter_by_person">Filter by Person</h2>
<div class="cam-filter">%s</div>
</div>

<div class="section">
<h2 data-i18n="summary_by_type">Summary by Type</h2>
<table>
<tr><th data-i18n="type_col">Type</th><th style="text-align:right" data-i18n="event_count">Event Count</th></tr>
%s
</table>
</div>

<div class="section">
<h2>%s</h2>
<table>
<tr>
  <th data-i18n="image">Image</th><th data-i18n="time">Time</th><th data-i18n="camera">Camera</th><th data-i18n="detected">Detected</th><th>Sub-Label</th><th data-i18n="confidence">Confidence</th>
  <th data-i18n="zone">Zone</th><th data-i18n="identity">Identity</th><th data-i18n="room">Room</th><th data-i18n="license_plate">License Plate</th><th data-i18n="brand_color">Brand/Color</th><th data-i18n="notes">Notes</th><th></th>
</tr>
%s
</table>
%s
</div>

` + feedbackModalHTML + `

<div class="links">
  <a href="/api/events">API: Events</a>
  <a href="/api/status">API: Status</a>
  <a href="/healthz">Health Check</a>
</div>
<p style="margin-top:1em;color:#666;font-size:.8em"><span data-i18n="auto_refresh">Auto-refresh every 10 seconds</span> | Storage: max 256 GB</p>

` + feedbackScript + `
` + i18nScript + `
<script>
var defaultTZ = '%s';
function getTimezone() { return localStorage.getItem('sg_tz') || defaultTZ; }
function setTimezone(tz) {
  localStorage.setItem('sg_tz', tz);
  // Reload with tz parameter
  var url = new URL(window.location);
  url.searchParams.set('tz', tz);
  window.location = url;
}
(function() {
  var sel = document.getElementById('tzSelect');
  if (sel) sel.value = getTimezone();
})();
</script>
</body>
</html>`

var eventsPageTpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SquareGuardian — Events</title>
<meta http-equiv="refresh" content="30">
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
.group-badge{position:absolute;bottom:22px;left:4px;background:rgba(156,39,176,.85);color:#fff;font-size:.6em;padding:2px 6px;border-radius:4px}
.more-indicator{display:flex;align-items:center;justify-content:center;width:150px;height:110px;background:#252836;border-radius:8px;color:#4fc3f7;font-size:.85em;cursor:default}

/* Multi-select mode */
.ev-checkbox{position:absolute;top:4px;left:4px;z-index:5;width:20px;height:20px;cursor:pointer;accent-color:#4fc3f7;display:none}
body.select-mode .ev-checkbox{display:block}
body.select-mode .ident-badge{left:28px}
.ev-card.selected{outline:3px solid #4fc3f7;outline-offset:-3px}
.ev-card.selected::after{content:'';position:absolute;inset:0;background:rgba(79,195,247,.15);pointer-events:none}

/* Select mode toggle */
.select-toggle{background:#252836;border:1px solid #555;color:#4fc3f7;padding:.4em 1em;border-radius:6px;cursor:pointer;font-size:.85em;margin-left:1em}
.select-toggle.active{background:#1976d2;color:#fff;border-color:#1976d2}

/* Floating action bar */
.select-bar{display:none;position:fixed;bottom:1.5em;left:50%%;transform:translateX(-50%%);background:#1a1d28;border:1px solid #4fc3f7;border-radius:12px;padding:.8em 1.5em;z-index:50;gap:1em;align-items:center;box-shadow:0 4px 24px rgba(0,0,0,.6)}
.select-bar.show{display:flex}
.select-bar .count{color:#4fc3f7;font-weight:bold;font-size:1em}
.select-bar button{padding:.4em 1em;border-radius:6px;border:none;cursor:pointer;font-size:.85em}
.select-bar .btn-group{background:#1976d2;color:#fff}
.select-bar .btn-group:hover{background:#1565c0}
.select-bar .btn-clear{background:#333;color:#ccc}
.select-bar .btn-clear:hover{background:#444}

/* Group modal */
.group-modal-bg{display:none;position:fixed;top:0;left:0;width:100%%;height:100%%;background:rgba(0,0,0,.7);z-index:100;justify-content:center;align-items:center}
.group-modal-bg.open{display:flex}
.group-modal{background:#1a1d28;border:1px solid #333;border-radius:12px;padding:1.5em;width:480px;max-width:95vw}
.group-modal h3{margin-bottom:1em;color:#fff}
.group-modal label{display:block;color:#999;font-size:.85em;margin-top:.8em}
.group-modal input{width:100%%;padding:.5em;background:#252836;border:1px solid #444;border-radius:6px;color:#e0e0e0;font-size:.9em;margin-top:.3em}
.group-modal .preview-grid{display:flex;gap:.5em;flex-wrap:wrap;margin-top:.8em;max-height:200px;overflow-y:auto}
.group-modal .preview-grid img{width:80px;height:60px;object-fit:cover;border-radius:4px}
.group-modal .actions{margin-top:1.2em;display:flex;gap:.8em;justify-content:flex-end}
.group-modal button{padding:.5em 1.2em;border-radius:6px;border:none;cursor:pointer;font-size:.9em}
.group-modal .btn-save{background:#1976d2;color:#fff}
.group-modal .btn-cancel{background:#333;color:#ccc}
.group-modal .msg{margin-top:.5em;font-size:.85em;color:#4caf50}

/* Groups summary */
.group-row{display:flex;align-items:center;gap:1em;background:#1a1d28;border-radius:8px;padding:.8em 1em;margin-bottom:.5em}
.group-info{flex:1}
.group-thumbs{display:flex;gap:4px}
.btn-delete-group{background:#333;border:1px solid #555;color:#f44336;padding:4px 10px;border-radius:4px;cursor:pointer;font-size:.8em}
.btn-delete-group:hover{background:#4a1a1a}
.top-bar{display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:.5em;margin-bottom:.5em}
.top-controls{display:flex;gap:.5em;align-items:center}
.lang-btn{background:#252836;border:1px solid #555;color:#4fc3f7;padding:4px 12px;border-radius:6px;cursor:pointer;font-size:.8em;font-weight:bold}
.lang-btn:hover{background:#333}
.tz-select{background:#252836;border:1px solid #555;color:#4fc3f7;padding:4px 8px;border-radius:6px;font-size:.8em}
</style>
</head>
<body>
<div class="top-bar">
  <h1>SquareGuardian — Events</h1>
  <div class="top-controls">
    <span style="color:#888;font-size:.8em" data-i18n="timezone">Timezone</span>
    <select class="tz-select" id="tzSelect" onchange="setTimezone(this.value)">%s</select>
    <button class="lang-btn" id="langToggle" onclick="toggleLang()">TH</button>
  </div>
</div>
<div class="nav">
  <a href="/" data-i18n="dashboard">Dashboard</a>
  <a href="/events" class="active" data-i18n="events_by_type">Events by Type</a>
  <a href="/faces" data-i18n="face_gallery">Face Gallery</a>
  <a href="/weights" data-i18n="weight_detection">Weight Detection</a>
  <button class="select-toggle" id="selectToggle" onclick="toggleSelectMode()" data-i18n="select_multiple">Select Multiple</button>
</div>

<p style="color:#888;font-size:.85em;margin-bottom:1em" data-i18n="click_to_identify">Click an image to identify, add room, license plate, brand/color — press "Select Multiple" to select multiple images and group them as the same person/vehicle</p>

%s

%s

` + feedbackModalHTML + `

<!-- Group Modal -->
<div class="group-modal-bg" id="groupModal">
<div class="group-modal">
  <h3>สร้างกลุ่มใหม่</h3>
  <p style="color:#888;font-size:.85em">รวมภาพที่เลือกเป็นกลุ่มเดียวกัน (เช่น คนเดียวกัน, รถคันเดียวกัน)</p>
  <label>ชื่อกลุ่ม</label>
  <input type="text" id="group-name" placeholder="เช่น สมชาย, รถขาว Toyota, คนส่งพัสดุ">
  <label>ประเภท</label>
  <input type="text" id="group-label" readonly style="color:#4fc3f7">
  <label>ภาพที่เลือก</label>
  <div class="preview-grid" id="group-preview"></div>
  <div class="actions">
    <button class="btn-cancel" onclick="closeGroupModal()">ยกเลิก</button>
    <button class="btn-save" onclick="saveGroup()">สร้างกลุ่ม</button>
  </div>
  <div class="msg" id="group-msg"></div>
</div>
</div>

<!-- Floating select bar -->
<div class="select-bar" id="selectBar">
  <span>เลือกแล้ว <span class="count" id="selectCount">0</span> รายการ</span>
  <button class="btn-group" onclick="openGroupModal()">สร้างกลุ่ม</button>
  <button class="btn-clear" onclick="clearSelection()">ยกเลิก</button>
</div>

<div class="links" style="margin-top:2em">
  <a href="/" data-i18n="dashboard">Dashboard</a>
  <a href="/api/events">API: Events</a>
  <a href="/api/training-data">API: Training Data</a>
  <a href="/api/groups">API: Groups</a>
</div>
<p style="margin-top:1em;color:#666;font-size:.8em">Auto-refresh 30s</p>

` + feedbackScript + `
` + i18nScript + `
<script>
var defaultTZ = '%s';
function getTimezone() { return localStorage.getItem('sg_tz') || defaultTZ; }
function setTimezone(tz) {
  localStorage.setItem('sg_tz', tz);
  var url = new URL(window.location);
  url.searchParams.set('tz', tz);
  window.location = url;
}
(function() {
  var sel = document.getElementById('tzSelect');
  if (sel) sel.value = getTimezone();
})();
</script>
<script>
var selectMode = false;
var selectedEvents = {};

function toggleSelectMode() {
  selectMode = !selectMode;
  document.body.classList.toggle('select-mode', selectMode);
  document.getElementById('selectToggle').classList.toggle('active', selectMode);
  if (!selectMode) clearSelection();
}

function handleCardClick(e, eventId, label, identity, room, plate, province, brand, color, note) {
  if (selectMode) {
    toggleSelect(eventId);
    return;
  }
  openFeedback(eventId, label, identity, room, plate, province, brand, color, note);
}

function toggleSelect(eventId) {
  if (selectedEvents[eventId]) {
    delete selectedEvents[eventId];
  } else {
    var card = document.querySelector('.ev-card[data-event-id="'+eventId+'"]');
    selectedEvents[eventId] = card ? card.getAttribute('data-label') : '';
  }
  updateSelectionUI();
}

function updateSelectionUI() {
  var count = Object.keys(selectedEvents).length;
  document.getElementById('selectCount').textContent = count;
  document.getElementById('selectBar').classList.toggle('show', count > 0);

  document.querySelectorAll('.ev-card').forEach(function(card) {
    var eid = card.getAttribute('data-event-id');
    var cb = card.querySelector('.ev-checkbox');
    if (selectedEvents[eid]) {
      card.classList.add('selected');
      if (cb) cb.checked = true;
    } else {
      card.classList.remove('selected');
      if (cb) cb.checked = false;
    }
  });
}

function clearSelection() {
  selectedEvents = {};
  updateSelectionUI();
}

function openGroupModal() {
  var ids = Object.keys(selectedEvents);
  if (ids.length < 2) {
    alert('กรุณาเลือกอย่างน้อย 2 รายการ');
    return;
  }
  // Determine label from first selected
  var labels = {};
  for (var id in selectedEvents) {
    labels[selectedEvents[id]] = (labels[selectedEvents[id]] || 0) + 1;
  }
  var topLabel = Object.keys(labels).sort(function(a,b){ return labels[b]-labels[a]; })[0];
  document.getElementById('group-label').value = topLabel;
  document.getElementById('group-name').value = '';
  document.getElementById('group-msg').textContent = '';

  // Show preview thumbnails
  var preview = document.getElementById('group-preview');
  preview.innerHTML = '';
  ids.forEach(function(eid) {
    var card = document.querySelector('.ev-card[data-event-id="'+eid+'"] img');
    if (card) {
      var img = document.createElement('img');
      img.src = card.src;
      preview.appendChild(img);
    }
  });

  document.getElementById('groupModal').classList.add('open');
}

function closeGroupModal() {
  document.getElementById('groupModal').classList.remove('open');
}

function saveGroup() {
  var name = document.getElementById('group-name').value.trim();
  var label = document.getElementById('group-label').value;
  var ids = Object.keys(selectedEvents);

  if (!name) {
    document.getElementById('group-msg').style.color = '#f44336';
    document.getElementById('group-msg').textContent = 'กรุณาใส่ชื่อกลุ่ม';
    return;
  }

  fetch('/api/group', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({name: name, label: label, event_ids: ids})
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.status === 'ok') {
      document.getElementById('group-msg').style.color = '#4caf50';
      document.getElementById('group-msg').textContent = 'สร้างกลุ่มสำเร็จ!';
      setTimeout(function() { closeGroupModal(); clearSelection(); toggleSelectMode(); location.reload(); }, 800);
    } else {
      document.getElementById('group-msg').style.color = '#f44336';
      document.getElementById('group-msg').textContent = 'Error: ' + (data.error || 'unknown');
    }
  })
  .catch(function(err) {
    document.getElementById('group-msg').style.color = '#f44336';
    document.getElementById('group-msg').textContent = 'Network error';
  });
}

function deleteGroup(groupId) {
  if (!confirm('ต้องการลบกลุ่มนี้?')) return;
  fetch('/api/groups/delete', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({group_id: groupId})
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.status === 'ok') location.reload();
    else alert('Error: ' + (data.error || 'unknown'));
  })
  .catch(function() { alert('Network error'); });
}

// Close group modal on background click
document.getElementById('groupModal').addEventListener('click', function(e) {
  if (e.target === this) closeGroupModal();
});
</script>
</body>
</html>`

var weightsPageTpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SquareGuardian — Weight Detection</title>
<style>` + sharedStyles + `
.top-bar{display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:.5em;margin-bottom:.5em}
.top-controls{display:flex;gap:.5em;align-items:center}
.lang-btn{background:#252836;border:1px solid #555;color:#4fc3f7;padding:4px 12px;border-radius:6px;cursor:pointer;font-size:.8em;font-weight:bold}
.lang-btn:hover{background:#333}
.tz-select{background:#252836;border:1px solid #555;color:#4fc3f7;padding:4px 8px;border-radius:6px;font-size:.8em}

/* Known identities grid */
.ident-grid{display:flex;gap:.8em;flex-wrap:wrap;padding:.5em 0}
.ident-card{background:#1a1d28;border:2px solid #333;border-radius:10px;padding:.6em;width:120px;text-align:center;cursor:pointer;transition:all .15s}
.ident-card:hover{border-color:#4fc3f7;transform:scale(1.05)}
.ident-card img{width:100px;height:80px;object-fit:cover;border-radius:6px;display:block;margin:0 auto .4em}
.ident-name{color:#4fc3f7;font-size:.85em;font-weight:bold;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ident-weight{color:#888;font-size:.7em;margin-top:2px}

/* Unidentified events grid */
.unident-grid{display:flex;flex-direction:column;gap:.8em}
.unident-card{display:flex;gap:1em;background:#1a1d28;border:1px solid #333;border-radius:10px;padding:.8em;align-items:center;flex-wrap:wrap}
.unident-thumb{width:120px;height:90px;object-fit:cover;border-radius:8px;flex-shrink:0}
.unident-info{min-width:140px}
.unident-time{color:#ccc;font-size:.85em}
.unident-camera{color:#888;font-size:.8em;margin-top:2px}
.unident-actions{flex:1;min-width:200px}
.suggest-label{color:#999;font-size:.8em;margin-bottom:.4em}
.suggest-btns{display:flex;gap:.4em;flex-wrap:wrap;margin-bottom:.5em}
.suggest-btn{background:#252836;border:1px solid #555;color:#4fc3f7;padding:4px 12px;border-radius:6px;cursor:pointer;font-size:.8em;transition:all .15s}
.suggest-btn:hover{background:#1976d2;color:#fff;border-color:#1976d2}
.suggest-btn.new-btn{color:#4caf50;border-color:#4caf50}
.suggest-btn.new-btn:hover{background:#2e7d32;color:#fff}
.suggest-weight{display:flex;align-items:center;gap:.5em;margin-top:.3em}
.suggest-weight label{color:#888;font-size:.8em;white-space:nowrap}
.weight-slider{width:120px;accent-color:#ff9800}
.weight-val{color:#ff9800;font-size:.8em;font-weight:bold;min-width:35px}

/* Label weight table */
.label-weight-slider{width:120px;accent-color:#4fc3f7}
.btn-save-weight{background:#252836;border:1px solid #555;color:#4fc3f7;padding:3px 12px;border-radius:4px;cursor:pointer;font-size:.8em}
.btn-save-weight:hover{background:#1976d2;color:#fff}

/* New identity modal */
.new-ident-modal-bg{display:none;position:fixed;top:0;left:0;width:100%%;height:100%%;background:rgba(0,0,0,.7);z-index:100;justify-content:center;align-items:center}
.new-ident-modal-bg.open{display:flex}
.new-ident-modal{background:#1a1d28;border:1px solid #333;border-radius:12px;padding:1.5em;width:400px;max-width:95vw}
.new-ident-modal h3{margin-bottom:1em;color:#fff}
.new-ident-modal label{display:block;color:#999;font-size:.85em;margin-top:.8em}
.new-ident-modal input{width:100%%;padding:.5em;background:#252836;border:1px solid #444;border-radius:6px;color:#e0e0e0;font-size:.9em;margin-top:.3em}
.new-ident-modal .actions{margin-top:1.2em;display:flex;gap:.8em;justify-content:flex-end}
.new-ident-modal button{padding:.5em 1.2em;border-radius:6px;border:none;cursor:pointer;font-size:.9em}
.new-ident-modal .btn-save{background:#1976d2;color:#fff}
.new-ident-modal .btn-cancel{background:#333;color:#ccc}
.new-ident-modal .msg{margin-top:.5em;font-size:.85em;color:#4caf50}

.success-flash{animation:flash .6s ease}
@keyframes flash{0%%{background:#1b5e20}100%%{background:transparent}}
</style>
</head>
<body>
<div class="top-bar">
  <h1 data-i18n="weight_detection_title">SquareGuardian — Weight Detection</h1>
  <div class="top-controls">
    <span style="color:#888;font-size:.8em" data-i18n="timezone">Timezone</span>
    <select class="tz-select" id="tzSelect" onchange="setTimezone(this.value)">%s</select>
    <button class="lang-btn" id="langToggle" onclick="toggleLang()">TH</button>
  </div>
</div>
<div class="nav">
  <a href="/" data-i18n="dashboard">Dashboard</a>
  <a href="/events" data-i18n="events_by_type">Events by Type</a>
  <a href="/faces" data-i18n="face_gallery">Face Gallery</a>
  <a href="/weights" class="active" data-i18n="weight_detection">Weight Detection</a>
</div>

<p style="color:#aaa;font-size:.9em;margin-bottom:1.2em" data-i18n="weight_page_desc">Suggest identities for unidentified persons. Suggested identities have lower confidence than manual annotations from the Dashboard.</p>

<div class="section">
  <h2 data-i18n="known_identities">Known Identities</h2>
  %s
</div>

<div class="section">
  <h2 data-i18n="unidentified_persons">Unidentified Persons <span style="color:#888;font-weight:normal;font-size:.85em">(%d)</span></h2>
  %s
</div>

<div class="section">
  <h2 data-i18n="label_weights">Detection Weights by Type</h2>
  <p style="color:#888;font-size:.8em;margin-bottom:.5em" data-i18n="label_weight_desc">Set default weights for each detection type. These are always lower than Frigate's actual detection score (max 49%%).</p>
  <table>
    <tr><th data-i18n="type_col">Type</th><th data-i18n="weight">Weight</th><th></th></tr>
    %s
  </table>
</div>

<!-- New Identity Modal -->
<div class="new-ident-modal-bg" id="newIdentModal">
<div class="new-ident-modal">
  <h3 data-i18n="add_new_identity">Add New Identity</h3>
  <input type="hidden" id="ni-event-id">
  <label data-i18n="identity_name">Identity (Name)</label>
  <input type="text" id="ni-name" placeholder="e.g. John, Delivery Person">
  <div class="suggest-weight" style="margin-top:.8em">
    <label data-i18n="confidence_level">Confidence:</label>
    <input type="range" min="10" max="90" value="50" class="weight-slider" id="ni-weight"
        oninput="document.getElementById('ni-wv').textContent=this.value+'%%'">
    <span id="ni-wv" class="weight-val">50%%</span>
  </div>
  <div class="actions">
    <button class="btn-cancel" onclick="closeNewIdentModal()">Cancel</button>
    <button class="btn-save" onclick="saveNewIdent()">Save</button>
  </div>
  <div class="msg" id="ni-msg"></div>
</div>
</div>

<div class="links" style="margin-top:2em">
  <a href="/" data-i18n="dashboard">Dashboard</a>
  <a href="/api/weights">API: Weights</a>
</div>

` + i18nScript + `
<script>
var defaultTZ = '%s';
function getTimezone() { return localStorage.getItem('sg_tz') || defaultTZ; }
function setTimezone(tz) {
  localStorage.setItem('sg_tz', tz);
  var url = new URL(window.location);
  url.searchParams.set('tz', tz);
  window.location = url;
}
(function() {
  var sel = document.getElementById('tzSelect');
  if (sel) sel.value = getTimezone();
})();

function suggestIdent(eventId, identity, defaultWeight) {
  var slider = document.getElementById('weight-' + eventId);
  var weight = slider ? parseInt(slider.value) : (defaultWeight * 100);
  fetch('/api/suggest-identity', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({event_id: eventId, identity: identity, weight: weight})
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.status === 'ok') {
      var card = document.querySelector('.unident-card[data-event-id="' + eventId + '"]');
      if (card) {
        card.classList.add('success-flash');
        card.querySelector('.suggest-label').textContent = '✓ ' + identity + ' (' + weight + '%%)';
        card.querySelector('.suggest-btns').style.display = 'none';
        card.querySelector('.suggest-weight').style.display = 'none';
      }
    } else {
      alert('Error: ' + (data.error || 'unknown'));
    }
  })
  .catch(function() { alert('Network error'); });
}

function addNewIdent(eventId) {
  document.getElementById('ni-event-id').value = eventId;
  document.getElementById('ni-name').value = '';
  document.getElementById('ni-weight').value = 50;
  document.getElementById('ni-wv').textContent = '50%%';
  document.getElementById('ni-msg').textContent = '';
  document.getElementById('newIdentModal').classList.add('open');
}

function closeNewIdentModal() {
  document.getElementById('newIdentModal').classList.remove('open');
}

function saveNewIdent() {
  var eventId = document.getElementById('ni-event-id').value;
  var name = document.getElementById('ni-name').value.trim();
  var weight = parseInt(document.getElementById('ni-weight').value);
  if (!name) {
    document.getElementById('ni-msg').style.color = '#f44336';
    document.getElementById('ni-msg').textContent = 'Please enter a name';
    return;
  }
  fetch('/api/suggest-identity', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({event_id: eventId, identity: name, weight: weight})
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.status === 'ok') {
      document.getElementById('ni-msg').style.color = '#4caf50';
      document.getElementById('ni-msg').textContent = 'Saved!';
      setTimeout(function() { closeNewIdentModal(); location.reload(); }, 600);
    } else {
      document.getElementById('ni-msg').style.color = '#f44336';
      document.getElementById('ni-msg').textContent = 'Error: ' + (data.error || 'unknown');
    }
  })
  .catch(function() {
    document.getElementById('ni-msg').style.color = '#f44336';
    document.getElementById('ni-msg').textContent = 'Network error';
  });
}

function saveLabelWeight(label, value) {
  fetch('/api/weights', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({label: label, weight: parseInt(value)})
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.status === 'ok') {
      var btn = event.target;
      btn.textContent = '✓';
      btn.style.color = '#4caf50';
      setTimeout(function() { btn.textContent = 'Save'; btn.style.color = ''; }, 1000);
    } else {
      alert('Error: ' + (data.error || 'unknown'));
    }
  })
  .catch(function() { alert('Network error'); });
}

// Close modal on background click
document.getElementById('newIdentModal').addEventListener('click', function(e) {
  if (e.target === this) closeNewIdentModal();
});
</script>
</body>
</html>`
