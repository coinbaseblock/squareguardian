# DATA_MODELS.md

## จุดประสงค์

เอกสารนี้กำหนดโครงข้อมูลที่ควรใช้กับ `home-guardian-ai` โดยตั้งใจให้:

- เริ่มง่าย
- เก็บเป็นไฟล์ JSON/YAML ได้
- ต่อไปฐานข้อมูลจริงได้
- รองรับการเพิ่มฟีเจอร์ในอนาคต

## หลักการ

- รอบแรกใช้ไฟล์ข้อมูลเรียบง่ายก่อน
- เก็บ schema ให้สอดคล้องกับ phase ถัดไป
- อย่าออกแบบ schema ใหญ่เกินการใช้งานจริงในวันแรก

## 1. Person Registry

โครงสร้างแนะนำ:

```text
data/
  identities/
    residents/
      resident_001/
        profile.json
        images/
    staff/
      staff_001/
        profile.json
        images/
    guests/
      guest_001/
        profile.json
        images/
    contractors/
      contractor_001/
        profile.json
        images/
```

ตัวอย่าง `profile.json`

```json
{
  "id": "staff_001",
  "name": "Somchai",
  "type": "staff",
  "active": true,
  "allowed_zones": ["office-main", "front-door"],
  "work_schedule": {
    "days": ["mon", "tue", "wed", "thu", "fri"],
    "start": "08:30",
    "end": "17:30"
  }
}
```

### ใช้จริงตอนนี้ได้แค่ไหน

**ใช้ได้เลยตอนนี้**
- เก็บรายชื่อคน
- เก็บประเภทคน
- เก็บ allowed zones
- เก็บ schedule

**ใช้ทีหลัง**
- face gallery images
- identity matching
- attendance by identity

## 2. Vehicle Registry

โครงสร้างแนะนำ:

```text
data/
  vehicles/
    staff/
      car_001/
        profile.json
        images/
    residents/
      car_002/
        profile.json
        images/
    guests/
      guest_car_001/
        profile.json
    suppliers/
      truck_001/
        profile.json
```

ตัวอย่าง `profile.json`

```json
{
  "id": "car_001",
  "plate_number": "1กข1234",
  "plate_normalized": "1กข1234",
  "owner_type": "staff",
  "owner_id": "staff_001",
  "vehicle_type": "sedan",
  "brand": "Toyota",
  "color": "white",
  "active": true,
  "allowed_zones": ["main-gate", "staff-parking"],
  "access_schedule": {
    "days": ["mon", "tue", "wed", "thu", "fri"],
    "start": "07:00",
    "end": "20:00"
  }
}
```

### ใช้จริงตอนนี้ได้แค่ไหน

**ใช้ได้เลยตอนนี้**
- เก็บทะเบียนรถ
- เก็บเจ้าของรถ
- เก็บ allowed zones / schedule

**ใช้ทีหลัง**
- LPR matching
- watchlist / blacklist
- owner auto-linking จาก OCR

## 3. Guest Mapping

ใช้กับโรงแรมหรือ visitor use case

โครงสร้างแนะนำ:

```text
data/
  guests/
    guest_001/
      profile.json
      face_images/
      vehicles/
```

ตัวอย่าง `profile.json`

```json
{
  "id": "guest_001",
  "name": "John Doe",
  "type": "hotel_guest",
  "room": "1208",
  "check_in_date": "2026-03-17",
  "check_out_date": "2026-03-19",
  "vehicles": ["2ขค5678"],
  "allowed_zones": ["lobby", "lift", "floor_12", "parking_guest"]
}
```

### ใช้จริงตอนนี้ได้แค่ไหน

**ตอนนี้ควรเก็บแค่ schema**

**ยังไม่ควรทำเต็ม flow ใน milestone แรก**

## 4. Event Schema

### Person event

```json
{
  "event_id": "evt_20260317_001",
  "camera": "front-door",
  "zone": "main-entrance",
  "event_type": "person_detected",
  "timestamp": "2026-03-17T08:41:15+07:00",
  "confidence": 0.93
}
```

### Presence / attendance event

```json
{
  "event_id": "evt_20260317_002",
  "person_id": "staff_001",
  "person_name": "Somchai",
  "camera": "front-door",
  "zone": "main-entrance",
  "direction": "in",
  "event_type": "check_in",
  "timestamp": "2026-03-17T08:41:15+07:00",
  "confidence": 0.93
}
```

### Vehicle event

```json
{
  "event_id": "veh_20260317_001",
  "camera": "gate-in",
  "zone": "main-gate",
  "event_type": "vehicle_entry",
  "vehicle_type": "sedan",
  "timestamp": "2026-03-17T08:12:05+07:00",
  "status": "observed"
}
```

### Vehicle + plate event

```json
{
  "event_id": "veh_20260317_002",
  "camera": "gate-in",
  "zone": "main-gate",
  "event_type": "vehicle_entry",
  "vehicle_type": "sedan",
  "plate_number": "1กข1234",
  "plate_confidence": 0.95,
  "owner_type": "staff",
  "owner_id": "staff_001",
  "timestamp": "2026-03-17T08:12:05+07:00",
  "status": "allowed"
}
```

### Alert event

```json
{
  "event_id": "alert_20260317_015",
  "camera": "warehouse-gate",
  "zone": "warehouse-entry",
  "event_type": "unauthorized_vehicle",
  "severity": "high",
  "timestamp": "2026-03-17T22:44:00+07:00"
}
```

## 5. Daily Attendance Summary

```json
{
  "person_id": "staff_001",
  "date": "2026-03-17",
  "check_in": "08:41:15",
  "check_out": "17:52:04",
  "work_duration_minutes": 550,
  "status": "late"
}
```

## 6. Model Registry Metadata

ยังไม่ต้องทำ model registry เต็มระบบ แต่ควรเผื่อ schema ไว้

```json
{
  "face_model": {
    "active": "gallery_v1",
    "previous": null
  },
  "behavior_model": {
    "active": "base_v1",
    "previous": null
  },
  "lpr_model": {
    "active": null,
    "previous": null
  }
}
```

### ใช้จริงตอนนี้ได้แค่ไหน

**ตอนนี้เก็บเป็น metadata file ไว้ก่อนได้**

**ยังไม่ต้องทำ rollback automation จริงในรอบแรก**

## 7. Alert Rule Schema

โครงตัวอย่าง:

```json
{
  "rule_id": "rule_frontdoor_loitering",
  "enabled": true,
  "event_type": "person_detected",
  "zone": "front-door",
  "condition": {
    "type": "loitering_seconds",
    "gte": 60
  },
  "action": {
    "notify": true,
    "severity": "medium"
  }
}
```

## 8. สรุปว่าอะไรใช้จริงก่อน

### ใช้จริงทันที

- person registry schema
- vehicle registry schema
- basic event schema
- alert rule schema
- attendance summary schema

### เก็บไว้เพื่อโตต่อ

- guest mapping schema
- model registry metadata
- full LPR event enrichment
- full hotel/factory access mapping

## แนวทางแนะนำ

เริ่มจากไฟล์ JSON/YAML ก่อน เพราะ:

- ดูง่าย
- debug ง่าย
- version control ได้
- เหมาะกับ MVP

เมื่อระบบนิ่งแล้วค่อยย้ายไปฐานข้อมูลจริง เช่น PostgreSQL หรือ SQLite โดย map schema เดิมต่อได้ง่าย
