package engine

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchSnapshot downloads the snapshot image for an event from Frigate.
func fetchSnapshot(frigateURL, eventID string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/api/events/%s/snapshot.jpg?crop=1&h=1080&quality=95", frigateURL, eventID)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapshot returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	return data, nil
}
