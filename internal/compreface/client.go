package compreface

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"time"
)

// Client communicates with CompreFace Recognition API.
type Client struct {
	baseURL   string
	apiKey    string
	threshold float64
	client    *http.Client
}

// FaceMatch represents a face recognition result from CompreFace.
type FaceMatch struct {
	Subject    string  `json:"subject"`
	Similarity float64 `json:"similarity"`
}

// RecognizeResult holds the full response from CompreFace /api/v1/recognition/recognize.
type RecognizeResult struct {
	Faces []FaceResult `json:"result"`
}

// FaceResult is a single face detected in the image.
type FaceResult struct {
	Box        Box         `json:"box"`
	Subjects   []FaceMatch `json:"subjects"`
	Age        *AgeResult  `json:"age,omitempty"`
	Gender     *GenderResult `json:"gender,omitempty"`
	Landmarks  [][]int     `json:"landmarks,omitempty"`
	ExecTimeMs int         `json:"execution_time"`
}

// Box is the bounding box of a detected face.
type Box struct {
	Probability float64 `json:"probability"`
	XMin        int     `json:"x_min"`
	YMin        int     `json:"y_min"`
	XMax        int     `json:"x_max"`
	YMax        int     `json:"y_max"`
}

// AgeResult from CompreFace age plugin.
type AgeResult struct {
	Probability float64 `json:"probability"`
	High        int     `json:"high"`
	Low         int     `json:"low"`
}

// GenderResult from CompreFace gender plugin.
type GenderResult struct {
	Probability float64 `json:"probability"`
	Value       string  `json:"value"`
}

// NewClient creates a CompreFace recognition client.
func NewClient(baseURL, apiKey string, threshold float64) *Client {
	if baseURL == "" || apiKey == "" {
		return nil
	}
	return &Client{
		baseURL:   baseURL,
		apiKey:    apiKey,
		threshold: threshold,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Recognize sends an image to CompreFace and returns matching subjects.
func (c *Client) Recognize(imageData []byte) (*RecognizeResult, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "snapshot.jpg")
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(imageData); err != nil {
		return nil, fmt.Errorf("write image: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("%s/api/v1/recognition/recognize?limit=5&det_prob_threshold=0.8&prediction_count=1", c.baseURL)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("compreface request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("compreface returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result RecognizeResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// AddSubject registers a new subject (person) with a face image.
func (c *Client) AddSubject(subject string, imageData []byte) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "face.jpg")
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(imageData); err != nil {
		return fmt.Errorf("write image: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("%s/api/v1/recognition/faces?subject=%s", c.baseURL, subject)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("add subject request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add subject returned %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[compreface] added subject %q", subject)
	return nil
}

// BestMatch returns the highest-similarity match above threshold, or nil.
func (c *Client) BestMatch(result *RecognizeResult) *FaceMatch {
	var best *FaceMatch
	for i := range result.Faces {
		for j := range result.Faces[i].Subjects {
			s := &result.Faces[i].Subjects[j]
			if s.Similarity >= c.threshold {
				if best == nil || s.Similarity > best.Similarity {
					best = s
				}
			}
		}
	}
	return best
}

// FaceCount returns the number of faces detected.
func FaceCount(result *RecognizeResult) int {
	return len(result.Faces)
}

// HasUnknown returns true if any face has no match above threshold.
func (c *Client) HasUnknown(result *RecognizeResult) bool {
	for _, face := range result.Faces {
		matched := false
		for _, s := range face.Subjects {
			if s.Similarity >= c.threshold {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
	}
	return len(result.Faces) > 0
}

// IsAvailable checks connectivity to CompreFace.
func (c *Client) IsAvailable() bool {
	resp, err := c.client.Get(c.baseURL + "/api/v1/recognition/subjects")
	if err != nil {
		return false
	}
	resp.Body.Close()
	// CompreFace returns 401 without api key, which means it's reachable.
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized
}
