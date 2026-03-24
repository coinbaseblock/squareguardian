package detector

import (
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

	// Persistent storage
	dataDir      string
	maxBytes     int64 // max storage bytes before cleanup
	saveInterval time.Duration
	dirty        bool

	// Face identification
	faceClient *FaceClient

	stopCh chan struct{}
}

// New creates a new Detector instance with disk-backed storage.
func New(frigateURL string, trackedItems []string, pollInterval time.Duration, dataDir string, maxStorageGB, bufferGB, saveIntervalS int, faceServiceURL string) *Detector {
	items := make(map[string]bool, len(trackedItems))
	for _, item := range trackedItems {
		items[strings.TrimSpace(item)] = true
	}
	maxBytes := int64(maxStorageGB-bufferGB) * 1024 * 1024 * 1024
	if maxBytes < 0 {
		maxBytes = 0
	}
	return &Detector{
		frigateURL:   strings.TrimRight(frigateURL, "/"),
		trackedItems: items,
		pollInterval: pollInterval,
		client:       &http.Client{Timeout: 10 * time.Second},
		idSet:        make(map[string]bool),
		dataDir:      dataDir,
		maxBytes:     maxBytes,
		saveInterval: time.Duration(saveIntervalS) * time.Second,
		faceClient:   NewFaceClient(faceServiceURL),
		stopCh:       make(chan struct{}),
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
	url := fmt.Sprintf("%s/api/events?limit=50", d.frigateURL)
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
	for i := range raw {
		if !d.trackedItems[raw[i].Label] {
			continue
		}
		if d.idSet[raw[i].ID] {
			continue // deduplicate
		}
		ev := raw[i].ToEvent()
		d.events = append([]Event{ev}, d.events...)
		d.idSet[ev.ID] = true
		newEvents = append(newEvents, ev)
		added++
	}

	if added > 0 {
		d.dirty = true
		log.Printf("detector: fetched %d new events (%d total cached)", added, len(d.events))
	}
	d.mu.Unlock()

	// Face identification runs outside the lock to avoid blocking polls
	for i := range newEvents {
		if newEvents[i].Label == "person" {
			d.identifyNewEvent(&newEvents[i])
			if newEvents[i].Identity != "" || newEvents[i].Note != "" {
				d.mu.Lock()
				for j := range d.events {
					if d.events[j].ID == newEvents[i].ID {
						d.events[j].Identity = newEvents[i].Identity
						d.events[j].Note = newEvents[i].Note
						d.dirty = true
						break
					}
				}
				d.mu.Unlock()
			}
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
