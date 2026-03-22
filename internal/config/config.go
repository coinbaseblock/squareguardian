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

	// Storage
	EventLogPath  string
	MaxStorageGB  int // max disk usage in GB before cleanup (default 256)
	BufferGB      int // keep this much free space when cleaning (default 10)
	SaveIntervalS int // how often to save to disk in seconds (default 30)
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		FrigateURL:   getEnv("FRIGATE_URL", "http://frigate:5000"),
		ListenAddr:   getEnv("LISTEN_ADDR", ":8080"),
		PollInterval: getDuration("POLL_INTERVAL_SEC", 5),
		TrackedItems: getList("TRACKED_ITEMS", []string{
			"person", "car", "motorcycle", "bus", "truck",
			"backpack", "suitcase", "handbag",
		}),
		EventLogPath:  getEnv("EVENT_LOG_PATH", "/data/events"),
		MaxStorageGB:  getInt("MAX_STORAGE_GB", 256),
		BufferGB:      getInt("BUFFER_GB", 10),
		SaveIntervalS: getInt("SAVE_INTERVAL_SEC", 30),
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
