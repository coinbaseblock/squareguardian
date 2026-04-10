package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Notifier sends alerts to LINE Notify, Telegram, and webhooks.
type Notifier struct {
	lineToken    string
	tgBotToken   string
	tgChatID     string
	webhookURL   string
	client       *http.Client
}

// Config for the notification service.
type Config struct {
	LINEToken      string
	TelegramToken  string
	TelegramChatID string
	WebhookURL     string
}

// AlertPayload is the structured alert sent to channels.
type AlertPayload struct {
	EventID    string `json:"event_id"`
	Camera     string `json:"camera"`
	AlertType  string `json:"alert_type"` // unknown_person, fall, loiter, vehicle_unknown
	PersonName string `json:"person_name,omitempty"`
	Action     string `json:"action,omitempty"`
	Plate      string `json:"plate,omitempty"`
	Zone       string `json:"zone,omitempty"`
	Timestamp  string `json:"timestamp"`
	Message    string `json:"message"`
	Snapshot   string `json:"snapshot_url,omitempty"`
}

// New creates a Notifier with the given config.
func New(cfg Config) *Notifier {
	return &Notifier{
		lineToken:  cfg.LINEToken,
		tgBotToken: cfg.TelegramToken,
		tgChatID:   cfg.TelegramChatID,
		webhookURL: cfg.WebhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// HasAnyChannel returns true if at least one notification channel is configured.
func (n *Notifier) HasAnyChannel() bool {
	return n.lineToken != "" || (n.tgBotToken != "" && n.tgChatID != "") || n.webhookURL != ""
}

// Send dispatches the alert to all configured channels.
func (n *Notifier) Send(alert AlertPayload) {
	if n.lineToken != "" {
		go n.sendLINE(alert)
	}
	if n.tgBotToken != "" && n.tgChatID != "" {
		go n.sendTelegram(alert)
	}
	if n.webhookURL != "" {
		go n.sendWebhook(alert)
	}
}

func (n *Notifier) sendLINE(alert AlertPayload) {
	data := url.Values{}
	data.Set("message", "\n"+alert.Message)

	req, err := http.NewRequest("POST", "https://notify-api.line.me/api/notify", strings.NewReader(data.Encode()))
	if err != nil {
		log.Printf("[notify] LINE request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+n.lineToken)

	resp, err := n.client.Do(req)
	if err != nil {
		log.Printf("[notify] LINE send error: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[notify] LINE returned %d", resp.StatusCode)
	}
}

func (n *Notifier) sendTelegram(alert AlertPayload) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.tgBotToken)
	body := map[string]string{
		"chat_id":    n.tgChatID,
		"text":       alert.Message,
		"parse_mode": "HTML",
	}
	jsonBody, _ := json.Marshal(body)

	resp, err := n.client.Post(apiURL, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		log.Printf("[notify] Telegram send error: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[notify] Telegram returned %d", resp.StatusCode)
	}
}

func (n *Notifier) sendWebhook(alert AlertPayload) {
	jsonBody, _ := json.Marshal(alert)
	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		log.Printf("[notify] webhook send error: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[notify] webhook returned %d: %s", resp.StatusCode, string(body))
	}
}
