package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"squareguardian/internal/api"
	"squareguardian/internal/compreface"
	"squareguardian/internal/config"
	"squareguardian/internal/detector"
	"squareguardian/internal/engine"
	"squareguardian/internal/mqtt"
	"squareguardian/internal/notify"
	"squareguardian/internal/storage"
	"squareguardian/internal/ws"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("SquareGuardian starting...")

	cfg := config.Load()

	// ─── SQLite Store ───────────────────────────────────────────
	store, err := storage.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()

	// ─── WebSocket Hub ──────────────────────────────────────────
	hub := ws.NewHub()

	// ─── CompreFace Client ──────────────────────────────────────
	cfClient := compreface.NewClient(cfg.CompreFaceURL, cfg.CompreFaceAPIKey, cfg.CompreFaceThreshold)
	if cfClient != nil {
		log.Printf("CompreFace configured: %s", cfg.CompreFaceURL)
	}

	// ─── Notification Service ───────────────────────────────────
	notifier := notify.New(notify.Config{
		LINEToken:      cfg.LINENotifyToken,
		TelegramToken:  cfg.TelegramBotToken,
		TelegramChatID: cfg.TelegramChatID,
		WebhookURL:     cfg.WebhookURL,
	})
	if notifier.HasAnyChannel() {
		log.Println("Notification channels configured")
	}

	// ─── MQTT Subscriber ────────────────────────────────────────
	mqttSub := mqtt.NewSubscriber(mqtt.Config{
		BrokerURL:   cfg.MQTTBroker,
		ClientID:    "squareguardian",
		TopicPrefix: cfg.MQTTTopicPrefix,
	})

	// ─── Event Correlation Engine ───────────────────────────────
	eng := engine.New(engine.Config{
		Store:       store,
		MQTTSub:     mqttSub,
		CompreFace:  cfClient,
		Notifier:    notifier,
		Hub:         hub,
		FrigateURL:  cfg.FrigateURL,
		CooldownSec: cfg.AlertCooldownSec,
	})
	if err := eng.Start(); err != nil {
		log.Printf("MQTT engine start warning (will retry on reconnect): %v", err)
	}
	defer eng.Stop()

	// ─── Legacy Detector (polling-based, kept for backward compat) ──
	det := detector.New(
		cfg.FrigateURL,
		cfg.TrackedItems,
		cfg.PollInterval,
		cfg.EventLogPath,
		cfg.MaxStorageGB,
		cfg.BufferGB,
		cfg.SaveIntervalS,
		cfg.FaceServiceURL,
		cfg.SnapshotBurstCount,
		cfg.SnapshotBurstIntervalMS,
		cfg.AlertCooldownSec,
	)
	det.Start()
	defer det.Stop()

	// ─── HTTP Server ────────────────────────────────────────────
	// Legacy API (v1 — polling-based detector)
	handler := api.New(det, cfg.CameraZones, cfg.FaceServiceURL, cfg.Timezone)

	// Engine API (v2 — MQTT + SQLite + WebSocket)
	engineAPI := api.NewEngineAPI(store, hub)

	// Combine: legacy handler is http.Handler (ServeMux), add v2 routes
	mux := http.NewServeMux()
	engineAPI.RegisterRoutes(mux)

	// Fallback to legacy handler for all other routes
	combined := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if v2 route
		switch {
		case len(r.URL.Path) >= 7 && r.URL.Path[:7] == "/api/v2",
			r.URL.Path == "/ws":
			mux.ServeHTTP(w, r)
		default:
			handler.ServeHTTP(w, r)
		}
	})

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      combined,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	go func() {
		log.Printf("API server listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("SquareGuardian stopped")
}
