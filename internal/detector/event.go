package detector

// Event represents a detection event from Frigate.
type Event struct {
	ID        string  `json:"id"`
	Camera    string  `json:"camera"`
	Label     string  `json:"label"`
	TopScore  float64 `json:"top_score"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Zone      string  `json:"zone,omitempty"`
	Thumbnail string  `json:"thumbnail,omitempty"`
	Snapshot  string  `json:"snapshot,omitempty"`
}

// FrigateEvent is the raw event structure returned by Frigate API.
type FrigateEvent struct {
	ID        string   `json:"id"`
	Camera    string   `json:"camera"`
	Label     string   `json:"label"`
	TopScore  float64  `json:"top_score"`
	StartTime float64  `json:"start_time"`
	EndTime   *float64 `json:"end_time"`
	Zones     []string `json:"zones"`
	Thumbnail string   `json:"thumbnail"`
	HasSnapshot bool   `json:"has_snapshot"`
}

// ToEvent converts a FrigateEvent to our internal Event representation.
func (fe *FrigateEvent) ToEvent() Event {
	e := Event{
		ID:        fe.ID,
		Camera:    fe.Camera,
		Label:     fe.Label,
		TopScore:  fe.TopScore,
		StartTime: fe.StartTime,
		Thumbnail: fe.Thumbnail,
	}
	if fe.EndTime != nil {
		e.EndTime = *fe.EndTime
	}
	if len(fe.Zones) > 0 {
		e.Zone = fe.Zones[0]
	}
	if fe.HasSnapshot {
		e.Snapshot = fe.ID
	}
	return e
}
