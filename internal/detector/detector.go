package detector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Detector polls Frigate for detection events and persists them to disk.
type Detector struct {
	frigateURL   string
	trackedItems map[string]bool
	pollInterval time.Duration
	client       *http.Client

	mu     sync.RWMutex
	events []Event
	idSet  map[string]bool // deduplication set
	groups []EventGroup    // named groups of events

	// Active event tracking: events still in progress (no end_time yet).
	// We defer face identification until the event settles for a better snapshot.
	activeEvents map[string]float64 // eventID → first seen timestamp

	// Best snapshot captured while event is active (person still in frame).
	// Key: eventID, Value: sharpest snapshot captured so far from camera live feed.
	activeSnapshots map[string]snapshotCandidate

	// Persistent storage
	dataDir      string
	maxBytes     int64 // max storage bytes before cleanup
	saveInterval time.Duration
	dirty        bool

	// Face identification
	faceClient *FaceClient

	// Snapshot burst settings
	burstCount    int
	burstInterval time.Duration

	// Alert cooldown: camera:identity → last alert time
	cooldownMu  sync.RWMutex
	alertCooldown map[string]time.Time
	cooldownDur   time.Duration

	stopCh chan struct{}
}

// New creates a new Detector instance with disk-backed storage.
func New(frigateURL string, trackedItems []string, pollInterval time.Duration, dataDir string, maxStorageGB, bufferGB, saveIntervalS int, faceServiceURL string, burstCount, burstIntervalMS, alertCooldownSec int) *Detector {
	items := make(map[string]bool, len(trackedItems))
	for _, item := range trackedItems {
		items[strings.TrimSpace(item)] = true
	}
	maxBytes := int64(maxStorageGB-bufferGB) * 1024 * 1024 * 1024
	if maxBytes < 0 {
		maxBytes = 0
	}
	if burstCount < 1 {
		burstCount = 1
	}
	if burstIntervalMS < 100 {
		burstIntervalMS = 100
	}
	if alertCooldownSec < 0 {
		alertCooldownSec = 0
	}
	return &Detector{
		frigateURL:    strings.TrimRight(frigateURL, "/"),
		trackedItems:  items,
		pollInterval:  pollInterval,
		client:        &http.Client{Timeout: 10 * time.Second},
		idSet:         make(map[string]bool),
		activeEvents:    make(map[string]float64),
		activeSnapshots: make(map[string]snapshotCandidate),
		dataDir:       dataDir,
		maxBytes:      maxBytes,
		saveInterval:  time.Duration(saveIntervalS) * time.Second,
		faceClient:    NewFaceClient(faceServiceURL),
		burstCount:    burstCount,
		burstInterval: time.Duration(burstIntervalMS) * time.Millisecond,
		alertCooldown: make(map[string]time.Time),
		cooldownDur:   time.Duration(alertCooldownSec) * time.Second,
		stopCh:        make(chan struct{}),
	}
}

// Start begins polling Frigate for events and periodic disk saves.
func (d *Detector) Start() {
	if err := d.loadFromDisk(); err != nil {
		log.Printf("detector: load from disk: %v (starting fresh)", err)
	} else {
		log.Printf("detector: loaded %d events from disk", len(d.events))
	}

	log.Printf("detector: polling %s every %s for items: %v",
		d.frigateURL, d.pollInterval, d.trackedLabels())
	go d.pollLoop()
	go d.saveLoop()
}

// Stop signals the poll loop to stop and saves data to disk.
func (d *Detector) Stop() {
	close(d.stopCh)
	if err := d.saveToDisk(); err != nil {
		log.Printf("detector: final save error: %v", err)
	}
}

// Events returns all cached events, optionally filtered by label and/or camera.
func (d *Detector) Events(labelFilter string) []Event {
	return d.EventsFiltered(labelFilter, "")
}

// EventsFiltered returns cached events filtered by label and/or camera.
func (d *Detector) EventsFiltered(labelFilter, cameraFilter string) []Event {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if labelFilter == "" && cameraFilter == "" {
		result := make([]Event, len(d.events))
		copy(result, d.events)
		return result
	}

	var filtered []Event
	for _, e := range d.events {
		if labelFilter != "" && e.Label != labelFilter {
			continue
		}
		if cameraFilter != "" && e.Camera != cameraFilter {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// IdentifiedEvents returns events that have been identified, grouped by identity name.
// Each identity maps to a slice of events sorted by most recent first (up to limit per identity).
func (d *Detector) IdentifiedEvents(limit int) map[string][]Event {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string][]Event)
	for _, e := range d.events {
		if e.Identity == "" {
			continue
		}
		if limit > 0 && len(result[e.Identity]) >= limit {
			continue
		}
		result[e.Identity] = append(result[e.Identity], e)
	}
	return result
}

// Cameras returns a deduplicated, sorted list of camera names.
// It queries Frigate's config API for all configured cameras and merges
// with cameras seen in cached events, so cameras appear even before
// they generate any detection events.
func (d *Detector) Cameras() []string {
	seen := make(map[string]bool)
	var cameras []string

	// First, try to get cameras from Frigate config API.
	if fc := d.frigateCameras(); len(fc) > 0 {
		for _, name := range fc {
			if !seen[name] {
				seen[name] = true
				cameras = append(cameras, name)
			}
		}
	}

	// Also include cameras from cached events (fallback / extra coverage).
	d.mu.RLock()
	for _, e := range d.events {
		if e.Camera != "" && !seen[e.Camera] {
			seen[e.Camera] = true
			cameras = append(cameras, e.Camera)
		}
	}
	d.mu.RUnlock()

	sort.Strings(cameras)
	return cameras
}

// frigateCameras queries Frigate's /api/config and returns configured camera names.
func (d *Detector) frigateCameras() []string {
	url := fmt.Sprintf("%s/api/config", d.frigateURL)
	resp, err := d.client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var cfg struct {
		Cameras map[string]json.RawMessage `json:"cameras"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil
	}

	names := make([]string, 0, len(cfg.Cameras))
	for name := range cfg.Cameras {
		names = append(names, name)
	}
	return names
}

// Annotate updates an event's user-provided fields.
func (d *Detector) Annotate(eventID, identity, roomNumber, licensePlate, province, vehicleBrand, vehicleColor, vehicleInfo, note string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := range d.events {
		if d.events[i].ID == eventID {
			if identity != "" {
				d.events[i].Identity = identity
			}
			if roomNumber != "" {
				d.events[i].RoomNumber = roomNumber
			}
			if licensePlate != "" {
				d.events[i].LicensePlate = licensePlate
			}
			if province != "" {
				d.events[i].Province = province
			}
			if vehicleBrand != "" {
				d.events[i].VehicleBrand = vehicleBrand
			}
			if vehicleColor != "" {
				d.events[i].VehicleColor = vehicleColor
			}
			if vehicleInfo != "" {
				d.events[i].VehicleInfo = vehicleInfo
			}
			if note != "" {
				d.events[i].Note = note
			}
			d.dirty = true
			return true
		}
	}
	return false
}

// FrigateURL returns the configured Frigate base URL.
func (d *Detector) FrigateURL() string {
	return d.frigateURL
}

// TrackedLabels returns the list of tracked item labels.
func (d *Detector) TrackedLabels() []string {
	return d.trackedLabels()
}

func (d *Detector) trackedLabels() []string {
	labels := make([]string, 0, len(d.trackedItems))
	for label := range d.trackedItems {
		labels = append(labels, label)
	}
	return labels
}

// waitForFrigate blocks until Frigate's API responds or stop is signalled.
func (d *Detector) waitForFrigate() bool {
	url := fmt.Sprintf("%s/api/version", d.frigateURL)
	backoff := 2 * time.Second
	maxBackoff := 30 * time.Second

	for {
		resp, err := d.client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Println("detector: Frigate API is ready")
				return true
			}
		}
		log.Printf("detector: waiting for Frigate at %s (retry in %s)", d.frigateURL, backoff)

		select {
		case <-time.After(backoff):
			if backoff < maxBackoff {
				backoff = backoff * 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		case <-d.stopCh:
			return false
		}
	}
}

func (d *Detector) pollLoop() {
	if !d.waitForFrigate() {
		return
	}

	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	// Initial poll
	d.poll()

	for {
		select {
		case <-ticker.C:
			d.poll()
		case <-d.stopCh:
			log.Println("detector: stopped")
			return
		}
	}
}

func (d *Detector) saveLoop() {
	ticker := time.NewTicker(d.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.mu.RLock()
			dirty := d.dirty
			d.mu.RUnlock()
			if dirty {
				if err := d.saveToDisk(); err != nil {
					log.Printf("detector: save error: %v", err)
				}
			}
			d.checkDiskUsage()
		case <-d.stopCh:
			return
		}
	}
}

func (d *Detector) poll() {
	// Use include_thumbnails=0 for faster polling; thumbnails are fetched on demand.
	url := fmt.Sprintf("%s/api/events?limit=50&include_thumbnails=0", d.frigateURL)
	resp, err := d.client.Get(url)
	if err != nil {
		log.Printf("detector: poll error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("detector: frigate returned status %d", resp.StatusCode)
		return
	}

	var raw []FrigateEvent
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		log.Printf("detector: decode error: %v", err)
		return
	}

	d.mu.Lock()

	added := 0
	var newEvents []Event
	var settledEvents []Event // events that were active and now have end_time

	for i := range raw {
		if !d.trackedItems[raw[i].Label] {
			continue
		}

		// Check if this is an active event that just ended (settled).
		if _, wasActive := d.activeEvents[raw[i].ID]; wasActive {
			if raw[i].EndTime != nil {
				// Event settled — person left the frame, Frigate has the best snapshot now.
				delete(d.activeEvents, raw[i].ID)
				// Update the event's end time and score in our cache.
				for j := range d.events {
					if d.events[j].ID == raw[i].ID {
						d.events[j].EndTime = *raw[i].EndTime
						d.events[j].TopScore = raw[i].BestScore()
						if raw[i].HasSnapshot {
							d.events[j].Snapshot = raw[i].ID
						}
						settledEvents = append(settledEvents, d.events[j])
						d.dirty = true
						break
					}
				}
			}
			continue
		}

		if d.idSet[raw[i].ID] {
			continue // already processed
		}

		ev := raw[i].ToEvent()
		d.events = append([]Event{ev}, d.events...)
		d.idSet[ev.ID] = true
		added++

		if raw[i].EndTime == nil {
			// Event is still active (person still in frame).
			// Track it and defer face identification until it settles.
			d.activeEvents[ev.ID] = ev.StartTime
			log.Printf("detector: tracking active event %s (%s on %s)", ev.ID, ev.Label, ev.Camera)
		} else {
			// Event already completed — process immediately.
			newEvents = append(newEvents, ev)
		}
	}

	// Clean up stale active events (older than 2 minutes with no update).
	now := float64(time.Now().Unix())
	for id, startTime := range d.activeEvents {
		if now-startTime > 120 {
			delete(d.activeEvents, id)
			// Find and add to settled for face identification
			for j := range d.events {
				if d.events[j].ID == id {
					settledEvents = append(settledEvents, d.events[j])
					break
				}
			}
			log.Printf("detector: active event %s timed out, processing now (has active snapshot: %v)", id, d.activeSnapshots[id].data != nil)
		}
	}

	// Capture live snapshots for active events (person still in frame).
	// This is key: we grab frames while the person is visible, not after they leave.
	activeCaptures := make(map[string]string) // eventID → camera name
	for id := range d.activeEvents {
		for j := range d.events {
			if d.events[j].ID == id {
				activeCaptures[id] = d.events[j].Camera
				break
			}
		}
	}

	if added > 0 {
		d.dirty = true
		log.Printf("detector: fetched %d new events (%d total, %d active)", added, len(d.events), len(d.activeEvents))
	}
	d.mu.Unlock()

	// Capture live camera snapshots for active events outside the lock.
	// Uses /api/{camera}/latest.jpg which returns a fresh frame each time.
	for eventID, camera := range activeCaptures {
		d.captureActiveSnapshot(eventID, camera)
	}

	// Process completed events and settled events for face identification.
	// Settled events get burst snapshot selection for the best image quality.
	allToProcess := append(newEvents, settledEvents...)
	for i := range allToProcess {
		if allToProcess[i].Label == "person" {
			d.identifyPersonEvent(&allToProcess[i])
		}
	}
}

// captureActiveSnapshot fetches a live frame from the camera and keeps the
// sharpest one seen so far for an active event. Called every poll cycle while
// the person is still in the frame — this is when we get the best images.
func (d *Detector) captureActiveSnapshot(eventID, camera string) {
	url := fmt.Sprintf("%s/api/%s/latest.jpg?h=720", d.frigateURL, camera)
	resp, err := d.client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return
	}
	data := buf.Bytes()
	score := imageSharpness(data)

	d.mu.Lock()
	defer d.mu.Unlock()

	prev, exists := d.activeSnapshots[eventID]
	if !exists || score > prev.sharpness {
		d.activeSnapshots[eventID] = snapshotCandidate{data: data, sharpness: score}
		if exists {
			log.Printf("detector: active snapshot for %s improved: %.1f → %.1f", eventID, prev.sharpness, score)
		} else {
			log.Printf("detector: active snapshot for %s captured (sharpness=%.1f)", eventID, score)
		}
	}
}

// identifyPersonEvent runs face identification on a person event using
// the best snapshot from burst captures and active-phase captures.
func (d *Detector) identifyPersonEvent(ev *Event) {
	if d.faceClient == nil || ev.Snapshot == "" {
		return
	}

	// Retrieve the best snapshot captured while the event was active.
	d.mu.Lock()
	activeBest, hasActive := d.activeSnapshots[ev.ID]
	delete(d.activeSnapshots, ev.ID)
	d.mu.Unlock()

	// Use burst snapshot selection: fetch multiple cropped snapshots from
	// Frigate and pick the sharpest. The burst function requests cropped images
	// focused on the detected person for better face detection.
	burstSnapshot, err := fetchBestSnapshot(d.frigateURL, ev.ID, d.burstCount, d.burstInterval)
	if err != nil && !hasActive {
		log.Printf("detector: face-id: no snapshot for %s: %v", ev.ID, err)
		return
	}

	// Pick the sharpest between active-phase captures and burst snapshot.
	var snapshot []byte
	if err == nil {
		burstScore := imageSharpness(burstSnapshot)
		if hasActive && activeBest.sharpness > burstScore {
			snapshot = activeBest.data
			log.Printf("detector: face-id %s: using active snapshot (%.1f) over burst snapshot (%.1f)",
				ev.ID, activeBest.sharpness, burstScore)
		} else {
			snapshot = burstSnapshot
			if hasActive {
				log.Printf("detector: face-id %s: using burst snapshot (%.1f) over active snapshot (%.1f)",
					ev.ID, burstScore, activeBest.sharpness)
			}
		}
	} else {
		snapshot = activeBest.data
		log.Printf("detector: face-id %s: using active snapshot (%.1f), burst snapshot failed",
			ev.ID, activeBest.sharpness)
	}

	result, err := d.faceClient.Identify(snapshot, ev.ID)
	if err != nil {
		log.Printf("detector: face-id: identify %s: %v", ev.ID, err)
		return
	}

	identified := false
	cooldownKey := ev.Camera + ":" + ev.ID
	if len(result.Matches) == 0 {
		// Mark as unknown outsider if faces were detected but none matched,
		// OR if no faces were detected at all (person too far / facing away).
		if result.HasUnknown || result.FacesDetected == 0 {
			ev.Identity = "คนภายนอก"
			if result.FacesDetected > 0 {
				ev.Note = "auto: ตรวจพบคนภายนอก (ไม่ตรงกับบุคคลที่ลงทะเบียน)"
			} else {
				ev.Note = "auto: ตรวจพบบุคคล (ไม่สามารถตรวจจับใบหน้าได้)"
			}
			log.Printf("detector: face-id: %s → unknown person (faces_detected=%d)", ev.ID, result.FacesDetected)
			identified = true
			cooldownKey = ev.Camera + ":unknown"
		}
	} else {
		best := result.Matches[0]
		if best.Status == "match" {
			ev.Identity = best.Name
			ev.Note = fmt.Sprintf("auto: ระบุตัวตน %s (%.0f%%)", best.Name, best.Similarity*100)
			log.Printf("detector: face-id: %s → %s (%.2f)", ev.ID, best.Name, best.Similarity)
			identified = true
			cooldownKey = ev.Camera + ":" + best.Name
		} else if best.Status == "suggest" {
			ev.Note = fmt.Sprintf("auto: อาจเป็น %s (%.0f%%)", best.Name, best.Similarity*100)
			log.Printf("detector: face-id: %s → suggest %s (%.2f)", ev.ID, best.Name, best.Similarity)
		}
	}

	// Update the event in cache.
	if ev.Identity != "" || ev.Note != "" {
		d.mu.Lock()
		for j := range d.events {
			if d.events[j].ID == ev.ID {
				d.events[j].Identity = ev.Identity
				d.events[j].Note = ev.Note
				d.dirty = true
				break
			}
		}
		d.mu.Unlock()
	}

	// Set cooldown per identity (not per camera) to avoid blocking
	// identification of different people on the same camera.
	if identified && d.cooldownDur > 0 {
		d.setCooldown(cooldownKey)
	}
}

// isOnCooldown checks if the given camera+identity pair recently had an alert.
// Uses exact key match so different persons on the same camera are not blocked.
func (d *Detector) isOnCooldown(key string) bool {
	if d.cooldownDur <= 0 {
		return false
	}
	d.cooldownMu.RLock()
	defer d.cooldownMu.RUnlock()

	lastAlert, exists := d.alertCooldown[key]
	if !exists {
		return false
	}
	return time.Since(lastAlert) < d.cooldownDur
}

// setCooldown records an alert time for cooldown tracking.
func (d *Detector) setCooldown(key string) {
	d.cooldownMu.Lock()
	defer d.cooldownMu.Unlock()
	d.alertCooldown[key] = time.Now()

	// Clean up old cooldown entries.
	now := time.Now()
	for k, t := range d.alertCooldown {
		if now.Sub(t) > d.cooldownDur*2 {
			delete(d.alertCooldown, k)
		}
	}
}

// loadFromDisk reads persisted events from the data directory.
func (d *Detector) loadFromDisk() error {
	if d.dataDir == "" {
		return nil
	}
	filePath := filepath.Join(d.dataDir, "events.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", filePath, err)
	}
	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		return fmt.Errorf("unmarshal events: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = events
	d.idSet = make(map[string]bool, len(events))
	for _, e := range events {
		d.idSet[e.ID] = true
	}

	// Load groups
	groupsPath := filepath.Join(d.dataDir, "groups.json")
	gdata, err := os.ReadFile(groupsPath)
	if err == nil {
		var groups []EventGroup
		if err := json.Unmarshal(gdata, &groups); err == nil {
			d.groups = groups
			log.Printf("detector: loaded %d groups from disk", len(groups))
		}
	}
	return nil
}

// saveToDisk persists all events to the data directory.
func (d *Detector) saveToDisk() error {
	if d.dataDir == "" {
		return nil
	}
	if err := os.MkdirAll(d.dataDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", d.dataDir, err)
	}

	d.mu.Lock()
	d.dirty = false
	eventsCopy := make([]Event, len(d.events))
	copy(eventsCopy, d.events)
	d.mu.Unlock()

	data, err := json.Marshal(eventsCopy)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	filePath := filepath.Join(d.dataDir, "events.json")
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("rename %s: %w", filePath, err)
	}
	log.Printf("detector: saved %d events to disk", len(eventsCopy))

	// Save groups
	groupsCopy := make([]EventGroup, len(d.groups))
	copy(groupsCopy, d.groups)
	if len(groupsCopy) > 0 {
		gdata, err := json.Marshal(groupsCopy)
		if err == nil {
			groupsPath := filepath.Join(d.dataDir, "groups.json")
			gtmpPath := groupsPath + ".tmp"
			if err := os.WriteFile(gtmpPath, gdata, 0644); err == nil {
				os.Rename(gtmpPath, groupsPath)
			}
		}
	}
	return nil
}

// GroupEvents assigns events to a named group.
func (d *Detector) GroupEvents(groupName, label string, eventIDs []string) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	groupID := fmt.Sprintf("g-%d", time.Now().UnixNano())

	// Set GroupID on matching events
	for i := range d.events {
		for _, id := range eventIDs {
			if d.events[i].ID == id {
				d.events[i].GroupID = groupID
			}
		}
	}

	d.groups = append(d.groups, EventGroup{
		ID:        groupID,
		Name:      groupName,
		Label:     label,
		EventIDs:  eventIDs,
		CreatedAt: float64(time.Now().Unix()),
	})
	d.dirty = true
	return groupID
}

// Groups returns all event groups.
func (d *Detector) Groups() []EventGroup {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]EventGroup, len(d.groups))
	copy(result, d.groups)
	return result
}

// DeleteGroup removes a group and unsets GroupID on its events.
func (d *Detector) DeleteGroup(groupID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	idx := -1
	for i, g := range d.groups {
		if g.ID == groupID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false
	}

	// Unset GroupID on events
	for i := range d.events {
		if d.events[i].GroupID == groupID {
			d.events[i].GroupID = ""
		}
	}

	d.groups = append(d.groups[:idx], d.groups[idx+1:]...)
	d.dirty = true
	return true
}

// TrainingData returns grouped events organized for model training export.
func (d *Detector) TrainingData() map[string][]Event {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string][]Event)
	for _, g := range d.groups {
		for _, e := range d.events {
			if e.GroupID == g.ID {
				result[g.ID] = append(result[g.ID], e)
			}
		}
	}
	return result
}

// checkDiskUsage removes oldest events when storage exceeds the limit.
func (d *Detector) checkDiskUsage() {
	if d.dataDir == "" || d.maxBytes <= 0 {
		return
	}
	var totalSize int64
	filepath.WalkDir(d.dataDir, func(path string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return nil
		}
		info, err := de.Info()
		if err != nil {
			return nil
		}
		totalSize += info.Size()
		return nil
	})

	if totalSize <= d.maxBytes {
		return
	}

	// Remove oldest 10% of events to free space
	d.mu.Lock()
	n := len(d.events)
	cutoff := n / 10
	if cutoff < 100 {
		cutoff = 100
	}
	if cutoff > n {
		cutoff = n
	}
	removed := d.events[n-cutoff:]
	d.events = d.events[:n-cutoff]
	for _, e := range removed {
		delete(d.idSet, e.ID)
	}
	d.dirty = true
	d.mu.Unlock()

	log.Printf("detector: disk usage %.1f GB > limit %.1f GB, removed %d oldest events",
		float64(totalSize)/(1024*1024*1024), float64(d.maxBytes)/(1024*1024*1024), cutoff)
	if err := d.saveToDisk(); err != nil {
		log.Printf("detector: save after cleanup error: %v", err)
	}
}
