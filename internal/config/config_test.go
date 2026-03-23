package config

import "testing"

func TestLoadCameraZonesFromYAMLReadsFirstZonePerCamera(t *testing.T) {
	data := []byte(`cameras:
  cam_front:
    zones:
      front_door:
        coordinates: 0,0,1,1
      backup:
        coordinates: 0,0,1,1
  cam_gate:
    zones:
      gate:
        coordinates: 0,0,1,1
`)

	got, err := loadCameraZonesFromYAML(data)
	if err != nil {
		t.Fatalf("loadCameraZonesFromYAML() error = %v", err)
	}
	if got["cam_front"] != "front_door" {
		t.Fatalf("cam_front zone = %q, want %q", got["cam_front"], "front_door")
	}
	if got["cam_gate"] != "gate" {
		t.Fatalf("cam_gate zone = %q, want %q", got["cam_gate"], "gate")
	}
}

func TestLoadCameraZonesFromYAMLReturnsEmptyWithoutZones(t *testing.T) {
	got, err := loadCameraZonesFromYAML([]byte(`cameras:
  cam_front:
    detect:
      fps: 5
`))
	if err != nil {
		t.Fatalf("loadCameraZonesFromYAML() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %#v", got)
	}
}
