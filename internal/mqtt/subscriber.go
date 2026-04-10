package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

// FrigatePayload represents the JSON payload from Frigate MQTT events.
type FrigatePayload struct {
	Type   string          `json:"type"`   // "new", "update", "end"
	Before *FrigateObject  `json:"before"` // state before the change
	After  *FrigateObject  `json:"after"`  // current state
}

// FrigateObject represents a tracked object in Frigate.
type FrigateObject struct {
	ID              string   `json:"id"`
	Camera          string   `json:"camera"`
	FrameTime       float64  `json:"frame_time"`
	SnapshotTime    float64  `json:"snapshot_time"`
	Label           string   `json:"label"`
	SubLabel        []string `json:"sub_label"`
	TopScore        float64  `json:"top_score"`
	Score           float64  `json:"score"`
	StartTime       float64  `json:"start_time"`
	EndTime         *float64 `json:"end_time"`
	Zones           []string `json:"zones"`
	CurrentZones    []string `json:"current_zones"`
	EnteredZones    []string `json:"entered_zones"`
	HasSnapshot     bool     `json:"has_snapshot"`
	HasClip         bool     `json:"has_clip"`
	Stationary      bool     `json:"stationary"`
	MotionlessCount int      `json:"motionless_count"`
	Region          []float64 `json:"region"`
	Box             []float64 `json:"box"`
	Area            float64  `json:"area"`
	Ratio           float64  `json:"ratio"`
}

// EventHandler is called when a Frigate event is received.
type EventHandler func(topic string, payload *FrigatePayload)

// Subscriber manages MQTT subscription to Frigate topics.
type Subscriber struct {
	client      paho.Client
	topicPrefix string
	handlers    []EventHandler
}

// Config holds MQTT connection settings.
type Config struct {
	BrokerURL   string // e.g. tcp://mosquitto:1883
	ClientID    string
	TopicPrefix string // e.g. "frigate"
}

// NewSubscriber creates an MQTT subscriber.
func NewSubscriber(cfg Config) *Subscriber {
	if cfg.ClientID == "" {
		cfg.ClientID = "squareguardian"
	}
	if cfg.TopicPrefix == "" {
		cfg.TopicPrefix = "frigate"
	}

	s := &Subscriber{
		topicPrefix: cfg.TopicPrefix,
	}

	opts := paho.NewClientOptions().
		AddBroker(cfg.BrokerURL).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(30 * time.Second).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Printf("[mqtt] connection lost: %v", err)
		}).
		SetOnConnectHandler(func(c paho.Client) {
			log.Printf("[mqtt] connected to %s", cfg.BrokerURL)
			s.subscribe(c)
		})

	s.client = paho.NewClient(opts)
	return s
}

// OnEvent registers a handler for Frigate events.
func (s *Subscriber) OnEvent(h EventHandler) {
	s.handlers = append(s.handlers, h)
}

// Start connects to the MQTT broker and begins subscribing.
func (s *Subscriber) Start() error {
	token := s.client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}
	return nil
}

// Stop disconnects from the broker.
func (s *Subscriber) Stop() {
	s.client.Disconnect(1000)
	log.Println("[mqtt] disconnected")
}

func (s *Subscriber) subscribe(c paho.Client) {
	// Subscribe to Frigate event topics: frigate/events
	// and per-camera review topics: frigate/+/+/+
	topics := map[string]byte{
		s.topicPrefix + "/events":   1,
		s.topicPrefix + "/reviews":  1,
		s.topicPrefix + "/+/person": 0, // per-camera person count
		s.topicPrefix + "/+/car":    0, // per-camera car count
	}

	for topic, qos := range topics {
		t := s.client.Subscribe(topic, qos, s.handleMessage)
		t.Wait()
		if err := t.Error(); err != nil {
			log.Printf("[mqtt] subscribe %s error: %v", topic, err)
		} else {
			log.Printf("[mqtt] subscribed to %s", topic)
		}
	}
}

func (s *Subscriber) handleMessage(_ paho.Client, msg paho.Message) {
	topic := msg.Topic()

	// Only process event payloads (JSON with before/after)
	if !strings.HasSuffix(topic, "/events") {
		return
	}

	var payload FrigatePayload
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		log.Printf("[mqtt] unmarshal error on %s: %v", topic, err)
		return
	}

	for _, h := range s.handlers {
		h(topic, &payload)
	}
}
