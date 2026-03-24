package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	// Frigate connection
	FrigateURL string

	// HTTP server
	ListenAddr string

	// Polling
	PollInterval time.Duration

	// Detection
	TrackedItems []string

	// Camera labeling derived from Frigate config
	CameraZones map[string]string

	// Storage
	EventLogPath  string
	MaxStorageGB  int // max disk usage in GB before cleanup (default 256)
	BufferGB      int // keep this much free space when cleaning (default 10)
	SaveIntervalS int // how often to save to disk in seconds (default 30)

	// Face service
	FaceServiceURL string

	// Snapshot burst: capture multiple snapshots and pick the sharpest one
	SnapshotBurstCount      int // number of snapshots per event (default 3)
	SnapshotBurstIntervalMS int // milliseconds between burst captures (default 500)

	// Alert cooldown: avoid re-alerting for the same person on the same camera
	AlertCooldownSec int // seconds to suppress duplicate alerts (default 300 = 5 min)

	// Timezone for display (default Asia/Bangkok)
	Timezone string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		FrigateURL:   getEnv("FRIGATE_URL", "http://frigate:5000"),
		ListenAddr:   getEnv("LISTEN_ADDR", ":8080"),
		PollInterval: getDuration("POLL_INTERVAL_SEC", 2),
		TrackedItems: getList("TRACKED_ITEMS", []string{
			"person", "car", "motorcycle", "bus", "truck",
			"backpack", "suitcase", "handbag",
		}),
		CameraZones:            loadCameraZones(),
		EventLogPath:           getEnv("EVENT_LOG_PATH", "/data/events"),
		MaxStorageGB:           getInt("MAX_STORAGE_GB", 256),
		BufferGB:               getInt("BUFFER_GB", 10),
		SaveIntervalS:          getInt("SAVE_INTERVAL_SEC", 30),
		FaceServiceURL:         getEnv("FACE_SERVICE_URL", ""),
		SnapshotBurstCount:     getInt("SNAPSHOT_BURST_COUNT", 3),
		SnapshotBurstIntervalMS: getInt("SNAPSHOT_BURST_INTERVAL_MS", 500),
		AlertCooldownSec:       getInt("ALERT_COOLDOWN_SEC", 300),
		Timezone:               getEnv("FRIGATE_TIMEZONE", "Asia/Bangkok"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getDuration(key string, fallbackSec int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return time.Duration(fallbackSec) * time.Second
}

func getList(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		return strings.Split(v, ",")
	}
	return fallback
}
