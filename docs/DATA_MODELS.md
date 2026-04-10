# DATA_MODELS.md

## จุดประสงค์

เอกสารนี้กำหนด schema ของ `SquareGuardian` ระบบใช้ **SQLite** (modernc.org/sqlite, pure Go) เป็น event store หลัก

## Database Schema (SQLite)

### 1. events — Unified Event (Who + What + Where + When)

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | Event ID (= Frigate event ID) |
| `frigate_id` | TEXT | Original Frigate event ID |
| `camera` | TEXT | Camera name (e.g. `cam_front`) |
| `label` | TEXT | Object label: person, car, motorcycle, etc. |
| `top_score` | REAL | Detection confidence |
| `start_time` | TEXT | ISO 8601 event start |
| `end_time` | TEXT | ISO 8601 event end |
| `zone` | TEXT | Zone name (e.g. `front_door`) |
| `person_id` | TEXT | **Who** — references `persons.id` |
| `person_name` | TEXT | **Who** — display name |
| `face_score` | REAL | Face recognition similarity |
| `action` | TEXT | **What** — stand, walk, run, fall, etc. |
| `action_score` | REAL | Action classification confidence |
| `vehicle_id` | TEXT | References `vehicles.id` |
| `plate` | TEXT | License plate text |
| `plate_province` | TEXT | Plate province |
| `snapshot_path` | TEXT | URL/path to snapshot image |
| `clip_path` | TEXT | URL/path to event clip |
| `thumbnail` | TEXT | Base64 thumbnail |
| `alert_sent` | INTEGER | 1 if alert was sent |
| `alert_type` | TEXT | unknown_person, fall, loiter, etc. |
| `created_at` | TEXT | ISO 8601 created timestamp |

### 2. persons — Person Registry

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | Unique person ID |
| `name` | TEXT | Display name |
| `role` | TEXT | resident, staff, visitor, unknown |
| `notes` | TEXT | Free-form notes |
| `created_at` | TEXT | ISO 8601 |

### 3. vehicles — Vehicle Registry

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | Unique vehicle ID |
| `license_plate` | TEXT | Plate number |
| `province` | TEXT | Province / region |
| `brand` | TEXT | Vehicle brand/model |
| `color` | TEXT | Vehicle color |
| `owner_id` | TEXT | References `persons.id` |
| `notes` | TEXT | Free-form notes |
| `created_at` | TEXT | ISO 8601 |

### 4. cameras — Camera Registry

| Column | Type | Description |
|--------|------|-------------|
| `name` | TEXT PK | Camera name (e.g. `cam_front`) |
| `rtsp_url` | TEXT | RTSP stream URL |
| `enabled` | INTEGER | 1 = active |
| `created_at` | TEXT | ISO 8601 |

### 5. alert_rules — Alert Configuration

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT PK | Rule ID |
| `name` | TEXT | Rule display name |
| `rule_type` | TEXT | unknown_person, fall, loiter, action, vehicle_unknown |
| `camera` | TEXT | Target camera (empty = all) |
| `zone` | TEXT | Target zone (empty = all) |
| `threshold` | REAL | Score threshold |
| `cooldown_s` | INTEGER | Seconds between re-alerts |
| `notify_line` | INTEGER | Send to LINE Notify |
| `notify_telegram` | INTEGER | Send to Telegram |
| `notify_webhook` | INTEGER | Send to webhook |
| `enabled` | INTEGER | 1 = active |
| `created_at` | TEXT | ISO 8601 |

## Event JSON Examples

### Person Detection + Face ID

```json
{
  "id": "1712750040.abcdef",
  "camera": "cam_front",
  "label": "person",
  "top_score": 0.92,
  "start_time": "2026-04-10T22:14:00Z",
  "end_time": "2026-04-10T22:14:30Z",
  "zone": "front_door",
  "person_name": "Somchai",
  "face_score": 0.95,
  "action": "walk",
  "action_score": 0.87,
  "alert_sent": false
}
```

### Unknown Person Alert

```json
{
  "id": "1712750100.xyz123",
  "camera": "cam_gate",
  "label": "person",
  "top_score": 0.88,
  "start_time": "2026-04-10T23:45:00Z",
  "zone": "gate",
  "person_name": "",
  "face_score": 0.0,
  "alert_sent": true,
  "alert_type": "unknown_person"
}
```

### Vehicle Detection

```json
{
  "id": "1712750200.veh001",
  "camera": "cam_parking",
  "label": "car",
  "top_score": 0.91,
  "start_time": "2026-04-10T08:30:00Z",
  "zone": "parking",
  "plate": "1กข1234",
  "plate_province": "กรุงเทพ",
  "vehicle_id": "v001"
}
```

### Action Recognition Event (MQTT payload from action service)

```json
{
  "camera": "cam_front",
  "action": "fall",
  "confidence": 0.78,
  "track_id": "cam_front",
  "timestamp": 1712750300.5
}
```

## Notification Alert Payload

```json
{
  "event_id": "1712750100.xyz123",
  "camera": "cam_gate",
  "alert_type": "unknown_person",
  "person_name": "",
  "action": "",
  "zone": "gate",
  "timestamp": "2026-04-10T23:45:00Z",
  "message": "Unknown person detected on cam_gate at 2026-04-10T23:45:00Z",
  "snapshot_url": "http://frigate:5000/api/events/1712750100.xyz123/snapshot.jpg"
}
```
