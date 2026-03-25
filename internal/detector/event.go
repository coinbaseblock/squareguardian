package detector

// Event represents a detection event from Frigate.
type Event struct {
	ID        string  `json:"id"`
	Camera    string  `json:"camera"`
	Label     string  `json:"label"`
	SubLabel  string  `json:"sub_label,omitempty"`
	TopScore  float64 `json:"top_score"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Zone      string  `json:"zone,omitempty"`
	Thumbnail string  `json:"thumbnail,omitempty"`
	Snapshot  string  `json:"snapshot,omitempty"`

	// User-provided annotations
	Identity     string `json:"identity,omitempty"`      // e.g. person name
	RoomNumber   string `json:"room_number,omitempty"`   // e.g. A0213
	LicensePlate string `json:"license_plate,omitempty"` // e.g. ABC 1234
	Province     string `json:"province,omitempty"`      // e.g. Bangkok
	VehicleBrand string `json:"vehicle_brand,omitempty"` // e.g. Toyota Camry
	VehicleColor string `json:"vehicle_color,omitempty"` // e.g. White
	VehicleInfo  string `json:"vehicle_info,omitempty"`  // legacy combined field
	Note         string `json:"note,omitempty"`          // free-form user feedback

	// Grouping for training data
	GroupID string `json:"group_id,omitempty"` // groups multiple events of the same entity
}

// EventGroup represents a named group of events belonging to the same entity.
type EventGroup struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Label    string `json:"label"`     // person, car, etc.
	EventIDs []string `json:"event_ids"`
	CreatedAt float64 `json:"created_at"`
}

// FrigateEvent is the raw event structure returned by Frigate API.
type FrigateEvent struct {
	ID          string   `json:"id"`
	Camera      string   `json:"camera"`
	Label       string   `json:"label"`
	SubLabel    *string  `json:"sub_label"`
	Score       float64  `json:"score"`     // current frame score
	TopScore    float64  `json:"top_score"` // highest score across all frames
	StartTime   float64  `json:"start_time"`
	EndTime     *float64 `json:"end_time"`
	Zones       []string `json:"zones"`
	Thumbnail   string   `json:"thumbnail"`
	HasSnapshot bool     `json:"has_snapshot"`
}

// BestScore returns the highest available confidence score.
func (fe *FrigateEvent) BestScore() float64 {
	if fe.TopScore > fe.Score {
		return fe.TopScore
	}
	return fe.Score
}

// ToEvent converts a FrigateEvent to our internal Event representation.
func (fe *FrigateEvent) ToEvent() Event {
	e := Event{
		ID:        fe.ID,
		Camera:    fe.Camera,
		Label:     fe.Label,
		TopScore:  fe.BestScore(),
		StartTime: fe.StartTime,
		Thumbnail: fe.Thumbnail,
	}
	if fe.SubLabel != nil {
		e.SubLabel = *fe.SubLabel
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
