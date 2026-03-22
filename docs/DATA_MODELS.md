# DATA_MODELS.md

## จุดประสงค์

เอกสารนี้กำหนด schema ขั้นต้นของ `SpaceGuardian` โดยตั้งใจให้เริ่มจากไฟล์ JSON/YAML ก่อน เพื่อให้ local pilot ทำงานง่ายและยังขยายเป็น service จริงในอนาคตได้.

## 1. Person Registry

```text
data/
  identities/
    residents/
      resident_001/
        profile.json
    staff/
      staff_001/
        profile.json
```

ตัวอย่าง `profile.json`

```json
{
  "id": "staff_001",
  "name": "Somchai",
  "type": "staff",
  "active": true,
  "allowed_zones": ["front-door", "office-main"],
  "work_schedule": {
    "days": ["mon", "tue", "wed", "thu", "fri"],
    "start": "08:30",
    "end": "17:30"
  }
}
```

## 2. Vehicle Registry

```text
data/
  vehicles/
    staff/
      car_001/
        profile.json
```

ตัวอย่าง `profile.json`

```json
{
  "id": "car_001",
  "plate_number": "1กข1234",
  "owner_type": "staff",
  "owner_id": "staff_001",
  "vehicle_type": "sedan",
  "color": "white",
  "active": true,
  "allowed_zones": ["main-gate", "staff-parking"]
}
```

## 3. Detection Event

```json
{
  "event_id": "evt_20260321_001",
  "camera": "cam_front",
  "zone": "front_door",
  "event_type": "person_detected",
  "timestamp": "2026-03-21T08:41:15+07:00",
  "confidence": 0.93
}
```

## 4. Vehicle Event

```json
{
  "event_id": "veh_20260321_001",
  "camera": "cam_front",
  "zone": "gate",
  "event_type": "vehicle_detected",
  "vehicle_type": "car",
  "timestamp": "2026-03-21T08:12:05+07:00",
  "status": "observed"
}
```

## 5. Alert Event

```json
{
  "event_id": "alert_20260321_015",
  "camera": "cam_front",
  "zone": "front_door",
  "event_type": "person_in_zone",
  "severity": "medium",
  "timestamp": "2026-03-21T22:44:00+07:00"
}
```

## สรุป

ในรอบแรก schema ควรเรียบง่ายพอที่จะ:

- เก็บ event ได้
- ผูก zone ได้
- ต่อยอด registry ได้
- ไม่บังคับให้ทำ identity/lpr ทันที
