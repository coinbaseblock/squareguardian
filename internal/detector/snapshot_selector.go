package detector

import (
	"fmt"
	"image"
	"image/jpeg"
	"bytes"
	"log"
	"math"
	"net/http"
	"time"
)

// snapshotCandidate holds a snapshot image with its sharpness score.
type snapshotCandidate struct {
	data      []byte
	sharpness float64
}

// fetchBestSnapshot fetches multiple snapshots for an event over a short burst
// window and returns the sharpest one. This helps avoid blurry captures when
// a person is moving quickly through the frame.
//
// burstCount: number of snapshots to attempt (e.g. 3)
// burstInterval: delay between attempts (e.g. 500ms)
func fetchBestSnapshot(frigateURL, eventID string, burstCount int, burstInterval time.Duration) ([]byte, error) {
	if burstCount <= 1 {
		return fetchSnapshot(frigateURL, eventID)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/api/events/%s/snapshot.jpg?crop=1&h=720&quality=95", frigateURL, eventID)

	var best snapshotCandidate
	fetched := 0

	for i := 0; i < burstCount; i++ {
		if i > 0 {
			time.Sleep(burstInterval)
		}

		resp, err := client.Get(url)
		if err != nil {
			log.Printf("detector: burst snapshot %d/%d fetch error: %v", i+1, burstCount, err)
			continue
		}

		data, err := readAndClose(resp)
		if err != nil {
			log.Printf("detector: burst snapshot %d/%d read error: %v", i+1, burstCount, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			continue
		}

		score := imageSharpness(data)
		fetched++

		if best.data == nil || score > best.sharpness {
			best = snapshotCandidate{data: data, sharpness: score}
		}
	}

	if best.data == nil {
		return nil, fmt.Errorf("all %d burst snapshot attempts failed for event %s", burstCount, eventID)
	}

	if fetched > 1 {
		log.Printf("detector: burst snapshot for %s: picked best of %d (sharpness=%.1f)", eventID, fetched, best.sharpness)
	}
	return best.data, nil
}

// readAndClose reads the full body and closes it.
func readAndClose(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(resp.Body)
	return buf.Bytes(), err
}

// imageSharpness calculates a sharpness score for a JPEG image using
// the Laplacian variance method. Higher values = sharper image.
// This is a simplified version that works on the grayscale luminance channel.
func imageSharpness(jpegData []byte) float64 {
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		// If we can't decode, use file size as a rough proxy
		// (sharper images compress to larger sizes).
		return float64(len(jpegData))
	}

	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

	if width < 3 || height < 3 {
		return 0
	}

	// Convert to grayscale luminance and compute Laplacian variance.
	// Laplacian kernel: [[0,1,0],[1,-4,1],[0,1,0]]
	// We sample every 2nd pixel for speed on large images.
	step := 1
	if width > 640 {
		step = 2
	}
	if width > 1280 {
		step = 3
	}

	var sum, sumSq float64
	count := 0

	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y += step {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x += step {
			// Laplacian = center*(-4) + top + bottom + left + right
			center := luminance(img, x, y)
			top := luminance(img, x, y-1)
			bottom := luminance(img, x, y+1)
			left := luminance(img, x-1, y)
			right := luminance(img, x+1, y)

			lap := -4*center + top + bottom + left + right
			sum += lap
			sumSq += lap * lap
			count++
		}
	}

	if count == 0 {
		return 0
	}

	// Variance of Laplacian
	mean := sum / float64(count)
	variance := sumSq/float64(count) - mean*mean
	return math.Abs(variance)
}

// luminance extracts grayscale brightness (0-255) from an image pixel.
func luminance(img image.Image, x, y int) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	// Standard luminance formula, values are 16-bit so divide by 257 for 8-bit.
	return 0.299*float64(r)/257 + 0.587*float64(g)/257 + 0.114*float64(b)/257
}
