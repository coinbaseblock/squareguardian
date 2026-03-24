package api

import (
	"testing"

	"squareguardian/internal/detector"
)

func TestDisplayZoneUsesEventZoneFirst(t *testing.T) {
	h := New(nil, map[string]string{"cam_front": "front_door"}, "", "Asia/Bangkok")
	e := detector.Event{Camera: "cam_front", Zone: "loading_bay"}

	if got := h.displayZone(e); got != "loading_bay" {
		t.Fatalf("displayZone() = %q, want %q", got, "loading_bay")
	}
}

func TestDisplayZoneFallsBackToCameraZoneMap(t *testing.T) {
	h := New(nil, map[string]string{"cam_front": "front_door"}, "", "Asia/Bangkok")
	e := detector.Event{Camera: "cam_front"}

	if got := h.displayZone(e); got != "front_door" {
		t.Fatalf("displayZone() = %q, want %q", got, "front_door")
	}
}

func TestDisplayZoneFallsBackToCameraName(t *testing.T) {
	h := New(nil, map[string]string{}, "", "Asia/Bangkok")
	e := detector.Event{Camera: "cam_loading"}

	if got := h.displayZone(e); got != "cam_loading" {
		t.Fatalf("displayZone() = %q, want %q", got, "cam_loading")
	}
}
