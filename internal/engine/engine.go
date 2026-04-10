package engine

import (
	"fmt"
	"log"
	"sync"
	"time"

	"squareguardian/internal/compreface"
	"squareguardian/internal/mqtt"
	"squareguardian/internal/notify"
	"squareguardian/internal/storage"
	"squareguardian/internal/ws"
)

// Engine is the central event correlation service.
// It subscribes to Frigate events via MQTT, enriches them with
// face identification (CompreFace) and action recognition,
// stores unified events in SQLite, broadcasts via WebSocket,
// and triggers alerts through the notification service.
type Engine struct {
	store       *storage.Store
	mqttSub     *mqtt.Subscriber
	compreface  *compreface.Client
	notifier    *notify.Notifier
	hub         *ws.Hub
	frigateURL  string

	// Alert cooldown: camera:label → last alert time
	cooldownMu  sync.RWMutex
	cooldowns   map[string]time.Time
	cooldownDur time.Duration
}

// Config for the Engine.
type Config struct {
	Store       *storage.Store
	MQTTSub     *mqtt.Subscriber
	CompreFace  *compreface.Client
	Notifier    *notify.Notifier
	Hub         *ws.Hub
	FrigateURL  string
	CooldownSec int
}

// New creates an Engine.
func New(cfg Config) *Engine {
	cooldown := time.Duration(cfg.CooldownSec) * time.Second
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}
	return &Engine{
		store:       cfg.Store,
		mqttSub:     cfg.MQTTSub,
		compreface:  cfg.CompreFace,
		notifier:    cfg.Notifier,
		hub:         cfg.Hub,
		frigateURL:  cfg.FrigateURL,
		cooldowns:   make(map[string]time.Time),
		cooldownDur: cooldown,
	}
}

// Start begins processing Frigate events from MQTT.
func (e *Engine) Start() error {
	// Register MQTT event handler
	e.mqttSub.OnEvent(e.handleFrigateEvent)

	if err := e.mqttSub.Start(); err != nil {
		return fmt.Errorf("start mqtt: %w", err)
	}

	log.Println("[engine] event correlation engine started")
	return nil
}

// Stop shuts down the engine.
func (e *Engine) Stop() {
	e.mqttSub.Stop()
	log.Println("[engine] stopped")
}

func (e *Engine) handleFrigateEvent(topic string, payload *mqtt.FrigatePayload) {
	if payload.After == nil {
		return
	}

	obj := payload.After
	eventType := payload.Type

	// Build unified event
	startTime := time.Unix(int64(obj.StartTime), int64((obj.StartTime-float64(int64(obj.StartTime)))*1e9))
	event := &storage.UnifiedEvent{
		ID:        obj.ID,
		FrigateID: obj.ID,
		Camera:    obj.Camera,
		Label:     obj.Label,
		TopScore:  obj.TopScore,
		StartTime: startTime.UTC().Format(time.RFC3339),
	}

	if obj.EndTime != nil {
		endTime := time.Unix(int64(*obj.EndTime), 0)
		event.EndTime = endTime.UTC().Format(time.RFC3339)
	}

	if len(obj.CurrentZones) > 0 {
		event.Zone = obj.CurrentZones[0]
	} else if len(obj.Zones) > 0 {
		event.Zone = obj.Zones[0]
	}

	if obj.HasSnapshot {
		event.SnapshotPath = fmt.Sprintf("%s/api/events/%s/snapshot.jpg", e.frigateURL, obj.ID)
	}

	// Enrich: face identification for person events
	if obj.Label == "person" && obj.HasSnapshot && e.compreface != nil {
		go e.identifyFace(event)
	}

	// Store event
	if err := e.store.InsertEvent(event); err != nil {
		log.Printf("[engine] store event error: %v", err)
	}

	// Broadcast to WebSocket clients
	e.hub.Broadcast(map[string]any{
		"type":  "event",
		"event": event,
		"mqtt_type": eventType,
	})

	// Check alert rules on "new" events
	if eventType == "new" {
		e.checkAlerts(event)
	}
}

func (e *Engine) identifyFace(event *storage.UnifiedEvent) {
	// Fetch snapshot from Frigate
	snapshot, err := fetchSnapshot(e.frigateURL, event.FrigateID)
	if err != nil {
		log.Printf("[engine] fetch snapshot for %s: %v", event.FrigateID, err)
		return
	}

	result, err := e.compreface.Recognize(snapshot)
	if err != nil {
		log.Printf("[engine] compreface recognize for %s: %v", event.FrigateID, err)
		return
	}

	match := e.compreface.BestMatch(result)
	if match != nil {
		event.PersonName = match.Subject
		event.FaceScore = match.Similarity
		if err := e.store.UpdateEventIdentity(event.ID, "", match.Subject, match.Similarity); err != nil {
			log.Printf("[engine] update identity error: %v", err)
		}
		log.Printf("[engine] identified %s on %s as %q (%.2f)", event.ID, event.Camera, match.Subject, match.Similarity)
	}

	// Unknown face alert
	if e.compreface.HasUnknown(result) {
		event.AlertType = "unknown_person"
		e.checkAlerts(event)
	}

	// Broadcast identity update
	e.hub.Broadcast(map[string]any{
		"type":  "identity_update",
		"event": event,
	})
}

func (e *Engine) checkAlerts(event *storage.UnifiedEvent) {
	if e.notifier == nil || !e.notifier.HasAnyChannel() {
		return
	}

	var alertType string
	var message string

	switch {
	case event.AlertType == "unknown_person":
		alertType = "unknown_person"
		message = fmt.Sprintf("Unknown person detected on %s at %s", event.Camera, event.StartTime)
	case event.Action == "fall":
		alertType = "fall"
		message = fmt.Sprintf("Fall detected on %s: %s at %s", event.Camera, event.PersonName, event.StartTime)
	case event.Label == "person" && event.PersonName == "":
		// Only alert for new unidentified persons
		return
	default:
		return
	}

	// Check cooldown
	cooldownKey := fmt.Sprintf("%s:%s", event.Camera, alertType)
	e.cooldownMu.RLock()
	lastAlert, exists := e.cooldowns[cooldownKey]
	e.cooldownMu.RUnlock()

	if exists && time.Since(lastAlert) < e.cooldownDur {
		return
	}

	e.cooldownMu.Lock()
	e.cooldowns[cooldownKey] = time.Now()
	e.cooldownMu.Unlock()

	alert := notify.AlertPayload{
		EventID:    event.ID,
		Camera:     event.Camera,
		AlertType:  alertType,
		PersonName: event.PersonName,
		Action:     event.Action,
		Zone:       event.Zone,
		Timestamp:  event.StartTime,
		Message:    message,
		Snapshot:   event.SnapshotPath,
	}

	e.notifier.Send(alert)

	if err := e.store.MarkAlertSent(event.ID, alertType); err != nil {
		log.Printf("[engine] mark alert error: %v", err)
	}

	log.Printf("[engine] alert sent: %s on %s", alertType, event.Camera)
}
