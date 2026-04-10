package storage

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database for event, person, and vehicle storage.
type Store struct {
	db *sql.DB
}

// Open creates or opens the SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	db.SetConnMaxLifetime(0)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	log.Printf("[store] SQLite database opened: %s", path)
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// UnifiedEvent is the correlated event combining Who + What + Where + When.
type UnifiedEvent struct {
	ID            string  `json:"id"`
	FrigateID     string  `json:"frigate_id"`
	Camera        string  `json:"camera"`
	Label         string  `json:"label"`
	TopScore      float64 `json:"top_score"`
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time"`
	Zone          string  `json:"zone"`
	PersonID      string  `json:"person_id,omitempty"`
	PersonName    string  `json:"person_name,omitempty"`
	FaceScore     float64 `json:"face_score,omitempty"`
	Action        string  `json:"action,omitempty"`
	ActionScore   float64 `json:"action_score,omitempty"`
	VehicleID     string  `json:"vehicle_id,omitempty"`
	Plate         string  `json:"plate,omitempty"`
	PlateProvince string  `json:"plate_province,omitempty"`
	SnapshotPath  string  `json:"snapshot_path,omitempty"`
	ClipPath      string  `json:"clip_path,omitempty"`
	Thumbnail     string  `json:"thumbnail,omitempty"`
	AlertSent     bool    `json:"alert_sent"`
	AlertType     string  `json:"alert_type,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

// InsertEvent stores a unified event.
func (s *Store) InsertEvent(e *UnifiedEvent) error {
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	alertSent := 0
	if e.AlertSent {
		alertSent = 1
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO events
		(id, frigate_id, camera, label, top_score, start_time, end_time, zone,
		 person_id, person_name, face_score,
		 action, action_score,
		 vehicle_id, plate, plate_province,
		 snapshot_path, clip_path, thumbnail,
		 alert_sent, alert_type, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.FrigateID, e.Camera, e.Label, e.TopScore, e.StartTime, e.EndTime, e.Zone,
		e.PersonID, e.PersonName, e.FaceScore,
		e.Action, e.ActionScore,
		e.VehicleID, e.Plate, e.PlateProvince,
		e.SnapshotPath, e.ClipPath, e.Thumbnail,
		alertSent, e.AlertType, e.CreatedAt,
	)
	return err
}

// UpdateEventIdentity updates the Who fields of an event.
func (s *Store) UpdateEventIdentity(eventID, personID, personName string, faceScore float64) error {
	_, err := s.db.Exec(`UPDATE events SET person_id=?, person_name=?, face_score=? WHERE id=?`,
		personID, personName, faceScore, eventID)
	return err
}

// UpdateEventAction updates the What fields of an event.
func (s *Store) UpdateEventAction(eventID, action string, actionScore float64) error {
	_, err := s.db.Exec(`UPDATE events SET action=?, action_score=? WHERE id=?`,
		action, actionScore, eventID)
	return err
}

// MarkAlertSent marks an event as having triggered an alert.
func (s *Store) MarkAlertSent(eventID, alertType string) error {
	_, err := s.db.Exec(`UPDATE events SET alert_sent=1, alert_type=? WHERE id=?`,
		alertType, eventID)
	return err
}

// QueryEvents returns events matching filters, ordered by start_time DESC.
func (s *Store) QueryEvents(camera, label string, limit, offset int) ([]UnifiedEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, frigate_id, camera, label, top_score, start_time, end_time, zone,
		person_id, person_name, face_score, action, action_score,
		vehicle_id, plate, plate_province, snapshot_path, clip_path, thumbnail,
		alert_sent, alert_type, created_at
		FROM events WHERE 1=1`
	args := []any{}

	if camera != "" {
		query += " AND camera=?"
		args = append(args, camera)
	}
	if label != "" {
		query += " AND label=?"
		args = append(args, label)
	}
	query += " ORDER BY start_time DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []UnifiedEvent
	for rows.Next() {
		var e UnifiedEvent
		var alertSent int
		if err := rows.Scan(
			&e.ID, &e.FrigateID, &e.Camera, &e.Label, &e.TopScore, &e.StartTime, &e.EndTime, &e.Zone,
			&e.PersonID, &e.PersonName, &e.FaceScore, &e.Action, &e.ActionScore,
			&e.VehicleID, &e.Plate, &e.PlateProvince, &e.SnapshotPath, &e.ClipPath, &e.Thumbnail,
			&alertSent, &e.AlertType, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		e.AlertSent = alertSent != 0
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetEvent returns a single event by ID.
func (s *Store) GetEvent(id string) (*UnifiedEvent, error) {
	var e UnifiedEvent
	var alertSent int
	err := s.db.QueryRow(`
		SELECT id, frigate_id, camera, label, top_score, start_time, end_time, zone,
		person_id, person_name, face_score, action, action_score,
		vehicle_id, plate, plate_province, snapshot_path, clip_path, thumbnail,
		alert_sent, alert_type, created_at
		FROM events WHERE id=?`, id).Scan(
		&e.ID, &e.FrigateID, &e.Camera, &e.Label, &e.TopScore, &e.StartTime, &e.EndTime, &e.Zone,
		&e.PersonID, &e.PersonName, &e.FaceScore, &e.Action, &e.ActionScore,
		&e.VehicleID, &e.Plate, &e.PlateProvince, &e.SnapshotPath, &e.ClipPath, &e.Thumbnail,
		&alertSent, &e.AlertType, &e.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.AlertSent = alertSent != 0
	return &e, nil
}

// Person represents a registered person.
type Person struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Notes     string `json:"notes,omitempty"`
	CreatedAt string `json:"created_at"`
}

// InsertPerson adds a person to the registry.
func (s *Store) InsertPerson(p *Person) error {
	if p.CreatedAt == "" {
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO persons (id, name, role, notes, created_at)
		VALUES (?,?,?,?,?)`, p.ID, p.Name, p.Role, p.Notes, p.CreatedAt)
	return err
}

// ListPersons returns all registered persons.
func (s *Store) ListPersons() ([]Person, error) {
	rows, err := s.db.Query(`SELECT id, name, role, notes, created_at FROM persons ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var persons []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.Role, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		persons = append(persons, p)
	}
	return persons, rows.Err()
}

// Vehicle represents a registered vehicle.
type Vehicle struct {
	ID           string `json:"id"`
	LicensePlate string `json:"license_plate"`
	Province     string `json:"province,omitempty"`
	Brand        string `json:"brand,omitempty"`
	Color        string `json:"color,omitempty"`
	OwnerID      string `json:"owner_id,omitempty"`
	Notes        string `json:"notes,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// InsertVehicle adds a vehicle to the registry.
func (s *Store) InsertVehicle(v *Vehicle) error {
	if v.CreatedAt == "" {
		v.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO vehicles
		(id, license_plate, province, brand, color, owner_id, notes, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		v.ID, v.LicensePlate, v.Province, v.Brand, v.Color, v.OwnerID, v.Notes, v.CreatedAt)
	return err
}

// ListVehicles returns all registered vehicles.
func (s *Store) ListVehicles() ([]Vehicle, error) {
	rows, err := s.db.Query(`SELECT id, license_plate, province, brand, color, owner_id, notes, created_at
		FROM vehicles ORDER BY license_plate`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vehicles []Vehicle
	for rows.Next() {
		var v Vehicle
		if err := rows.Scan(&v.ID, &v.LicensePlate, &v.Province, &v.Brand, &v.Color, &v.OwnerID, &v.Notes, &v.CreatedAt); err != nil {
			return nil, err
		}
		vehicles = append(vehicles, v)
	}
	return vehicles, rows.Err()
}

// PurgeOldEvents deletes events older than the given duration.
func (s *Store) PurgeOldEvents(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).UTC().Format(time.RFC3339Nano)
	result, err := s.db.Exec(`DELETE FROM events WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// EventCount returns total event count for stats.
func (s *Store) EventCount() (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count)
	return count, err
}
