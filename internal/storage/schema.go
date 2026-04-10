package storage

// DDL statements executed on database open.
const schema = `
CREATE TABLE IF NOT EXISTS cameras (
    name       TEXT PRIMARY KEY,
    rtsp_url   TEXT NOT NULL DEFAULT '',
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS persons (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT '',  -- e.g. resident, staff, visitor, unknown
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS vehicles (
    id            TEXT PRIMARY KEY,
    license_plate TEXT NOT NULL DEFAULT '',
    province      TEXT NOT NULL DEFAULT '',
    brand         TEXT NOT NULL DEFAULT '',
    color         TEXT NOT NULL DEFAULT '',
    owner_id      TEXT NOT NULL DEFAULT '',  -- references persons.id
    notes         TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS events (
    id           TEXT PRIMARY KEY,
    frigate_id   TEXT NOT NULL DEFAULT '',
    camera       TEXT NOT NULL DEFAULT '',
    label        TEXT NOT NULL DEFAULT '',        -- person, car, motorcycle, etc.
    top_score    REAL NOT NULL DEFAULT 0,
    start_time   TEXT NOT NULL DEFAULT '',
    end_time     TEXT NOT NULL DEFAULT '',
    zone         TEXT NOT NULL DEFAULT '',

    -- Who
    person_id    TEXT NOT NULL DEFAULT '',         -- references persons.id
    person_name  TEXT NOT NULL DEFAULT '',
    face_score   REAL NOT NULL DEFAULT 0,

    -- What
    action       TEXT NOT NULL DEFAULT '',         -- stand, walk, run, fall, etc.
    action_score REAL NOT NULL DEFAULT 0,

    -- Vehicle
    vehicle_id   TEXT NOT NULL DEFAULT '',         -- references vehicles.id
    plate        TEXT NOT NULL DEFAULT '',
    plate_province TEXT NOT NULL DEFAULT '',

    -- Metadata
    snapshot_path TEXT NOT NULL DEFAULT '',
    clip_path     TEXT NOT NULL DEFAULT '',
    thumbnail     TEXT NOT NULL DEFAULT '',

    -- Alert
    alert_sent   INTEGER NOT NULL DEFAULT 0,
    alert_type   TEXT NOT NULL DEFAULT '',         -- unknown_person, fall, loiter, etc.

    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_events_camera ON events(camera);
CREATE INDEX IF NOT EXISTS idx_events_label ON events(label);
CREATE INDEX IF NOT EXISTS idx_events_person_id ON events(person_id);
CREATE INDEX IF NOT EXISTS idx_events_start_time ON events(start_time);
CREATE INDEX IF NOT EXISTS idx_events_action ON events(action);
CREATE INDEX IF NOT EXISTS idx_events_frigate_id ON events(frigate_id);

CREATE TABLE IF NOT EXISTS alert_rules (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    rule_type   TEXT NOT NULL DEFAULT '',  -- unknown_person, fall, loiter, vehicle_unknown, action
    camera      TEXT NOT NULL DEFAULT '',  -- empty = all cameras
    zone        TEXT NOT NULL DEFAULT '',  -- empty = all zones
    threshold   REAL NOT NULL DEFAULT 0,
    cooldown_s  INTEGER NOT NULL DEFAULT 300,
    notify_line    INTEGER NOT NULL DEFAULT 0,
    notify_telegram INTEGER NOT NULL DEFAULT 0,
    notify_webhook  INTEGER NOT NULL DEFAULT 0,
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
`
