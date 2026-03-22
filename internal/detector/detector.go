package detector

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

	// Persistent storage
	dataDir      string
	maxBytes     int64 // max storage bytes before cleanup
	saveInterval time.Duration
	dirty        bool

	stopCh chan struct{}
}

// New creates a new Detector instance with disk-backed storage.
func New(frigateURL string, trackedItems []string, pollInterval time.Duration, dataDir string, maxStorageGB, bufferGB, saveIntervalS int) *Detector {
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

// Events returns all cached events, optionally filtered by label.
func (d *Detector) Events(labelFilter string) []Event {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if labelFilter == "" {
		result := make([]Event, len(d.events))
		copy(result, d.events)
		return result
	}

	var filtered []Event
	for _, e := range d.events {
		if e.Label == labelFilter {
			filtered = append(filtered, e)
		}
	}
	return filtered
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

func (d *Detector) pollLoop() {
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
	defer d.mu.Unlock()

	added := 0
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
		added++
	}

	if added > 0 {
		d.dirty = true
		log.Printf("detector: fetched %d new events (%d total cached)", added, len(d.events))
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
	return nil
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
