package config

import (
	"bufio"
	"bytes"
	"os"
	"strings"
)

var frigateConfigCandidates = []string{
	"/config/config.yml",
	"config/frigate/config.yml",
}

func loadCameraZones() map[string]string {
	for _, path := range frigateConfigCandidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		zones, err := loadCameraZonesFromYAML(data)
		if err != nil || len(zones) == 0 {
			continue
		}
		return zones
	}
	return map[string]string{}
}

func loadCameraZonesFromYAML(data []byte) (map[string]string, error) {
	zones := make(map[string]string)
	var currentCamera string
	inCameras := false
	inZones := false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))
		key := strings.TrimSuffix(trimmed, ":")

		if indent == 0 {
			inCameras = key == "cameras"
			if !inCameras {
				currentCamera = ""
				inZones = false
			}
			continue
		}
		if !inCameras {
			continue
		}
		if indent <= 2 {
			currentCamera = ""
			inZones = false
		}
		if indent == 2 && strings.HasSuffix(trimmed, ":") {
			currentCamera = key
			inZones = false
			continue
		}
		if currentCamera == "" {
			continue
		}
		if indent == 4 {
			inZones = key == "zones"
			continue
		}
		if inZones && indent == 6 && strings.HasSuffix(trimmed, ":") {
			if zones[currentCamera] == "" {
				zones[currentCamera] = key
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return zones, nil
}
