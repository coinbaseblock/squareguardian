package detector

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Detector polls Frigate for detection events and stores them in memory.
type Detector struct {
	frigateURL   string
	trackedItems map[string]bool
	pollInterval time.Duration
	client       *http.Client

	mu     sync.RWMutex
	events []Event
	// Keep last N events in memory
	maxEvents int

	stopCh chan struct{}
}

// New creates a new Detector instance.
func New(frigateURL string, trackedItems []string, pollInterval time.Duration) *Detector {
	items := make(map[string]bool, len(trackedItems))
	for _, item := range trackedItems {
		items[strings.TrimSpace(item)] = true
	}
	return &Detector{
		frigateURL:   strings.TrimRight(frigateURL, "/"),
		trackedItems: items,
		pollInterval: pollInterval,
		client:       &http.Client{Timeout: 10 * time.Second},
		maxEvents:    1000,
		stopCh:       make(chan struct{}),
	}
}

// Start begins polling Frigate for events.
func (d *Detector) Start() {
	log.Printf("detector: polling %s every %s for items: %v",
		d.frigateURL, d.pollInterval, d.trackedLabels())
	go d.pollLoop()
}

// Stop signals the poll loop to stop.
func (d *Detector) Stop() {
	close(d.stopCh)
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

	var newEvents []Event
	for i := range raw {
		if d.trackedItems[raw[i].Label] {
			newEvents = append(newEvents, raw[i].ToEvent())
		}
	}

	if len(newEvents) > 0 {
		d.mu.Lock()
		d.events = append(newEvents, d.events...)
		if len(d.events) > d.maxEvents {
			d.events = d.events[:d.maxEvents]
		}
		d.mu.Unlock()
		log.Printf("detector: fetched %d events (%d total cached)", len(newEvents), len(d.events))
	}
}
