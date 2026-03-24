package detector

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FaceClient communicates with the face-service for person identification.
type FaceClient struct {
	baseURL string
	client  *http.Client
}

// FaceMatch represents an identification result from the face-service.
type FaceMatch struct {
	PersonID   string  `json:"person_id"`
	Name       string  `json:"name"`
	Similarity float64 `json:"similarity"`
	Status     string  `json:"status"` // "match" or "suggest"
}

type identifyRequest struct {
	Image   string `json:"image"`
	EventID string `json:"event_id"`
}

type identifyResponse struct {
	Matches       []FaceMatch `json:"matches"`
	FacesDetected int         `json:"faces_detected"`
	HasUnknown    bool        `json:"has_unknown"`
}

// NewFaceClient creates a client for the face-service.
func NewFaceClient(baseURL string) *FaceClient {
	if baseURL == "" {
		return nil
	}
	return &FaceClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// IdentifyResult holds matches and unknown face info from face-service.
type IdentifyResult struct {
	Matches    []FaceMatch
	HasUnknown bool
}

// Identify sends a snapshot image to the face-service and returns matches.
func (fc *FaceClient) Identify(imageData []byte, eventID string) (*IdentifyResult, error) {
	b64 := base64.StdEncoding.EncodeToString(imageData)

	body, err := json.Marshal(identifyRequest{
		Image:   b64,
		EventID: eventID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := fc.client.Post(
		fc.baseURL+"/api/face/identify",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("face-service request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("face-service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result identifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &IdentifyResult{
		Matches:    result.Matches,
		HasUnknown: result.HasUnknown,
	}, nil
}

// IsAvailable checks if the face-service is reachable.
func (fc *FaceClient) IsAvailable() bool {
	resp, err := fc.client.Get(fc.baseURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// fetchSnapshot downloads the snapshot image for an event from Frigate.
func fetchSnapshot(frigateURL, eventID string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/api/events/%s/snapshot.jpg", frigateURL, eventID)
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

// identifyNewEvent is kept for backward compatibility but delegates to
// the Detector's identifyPersonEvent method which uses burst snapshots
// and alert cooldown. Callers should use identifyPersonEvent directly.

