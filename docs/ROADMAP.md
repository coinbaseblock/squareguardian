# ROADMAP.md

## เป้าหมาย

roadmap นี้จัดลำดับให้ `home-guardian-ai` โตจากระบบเฝ้าระวังพื้นฐาน ไปเป็นระบบ identity + vehicle + access intelligence โดยไม่เริ่มยากเกินไป

## Phase 0 — Repo Bootstrap

เป้าหมาย:

- ตั้งชื่อ repo และโครง docs ให้ชัด
- กำหนด MVP scope
- เลือก deployment path สำหรับ dev

สิ่งที่ควรมี:

- `README.md`
- `AGENTS.md`
- `docs/MVP-FIRST.md`
- `docs/ARCHITECTURE.md`
- `docs/ROADMAP.md`
- `docs/FEATURE_MATRIX.md`
- `docs/DATA_MODELS.md`

ผลลัพธ์:

- มีเอกสารพร้อมเปิด repo
- มีขอบเขตที่ชัดว่าอะไรทำก่อน

## Phase 1 — Detection Pilot

เป้าหมาย:

ให้ระบบใช้งานกับกล้องจริงได้

สิ่งที่ทำ:

- Docker Compose
- Frigate config สำหรับกล้อง 1 ตัว
- detect `person`
- detect `vehicle`
- snapshots / short clips
- zones / line crossing
- event log ขั้นต้น

ผลลัพธ์:

- ใช้งานเฝ้าระวังพื้นฐานได้จริง

## Phase 2 — Rule-based Security

เป้าหมาย:

ให้ระบบตอบเหตุการณ์สำคัญได้โดยยังไม่ต้องพึ่ง model เพิ่มมาก

สิ่งที่ทำ:

- front-door / gate / parking zones
- line crossing `in` / `out`
- rules สำหรับ:
  - loitering
  - after-hours movement
  - vehicle at gate
  - person in restricted zone
- notifier

ผลลัพธ์:

- เริ่มใช้งานแบบ alert-driven ได้

## Phase 3 — Registry Foundation

เป้าหมาย:

วางรากฐานสำหรับ known/unknown และ access rules

สิ่งที่ทำ:

- person registry
- vehicle registry
- schema สำหรับ guest / contractor / resident / staff
- event normalization

ผลลัพธ์:

- ระบบเริ่มรู้จัก metadata ของคนและรถ แม้ยังไม่ match อัตโนมัติเต็มรูปแบบ

## Phase 4 — Identity / Presence

เป้าหมาย:

เริ่มรู้ว่าใครเข้าออก

สิ่งที่ทำ:

- face gallery
- known / unknown person
- identity confidence
- attendance logic จาก line crossing + identity
- daily summary เบื้องต้น

ผลลัพธ์:

- ใช้งาน check-in / check-out แบบกล้องได้ในระดับ pilot

## Phase 5 — Vehicle / LPR

เป้าหมาย:

ให้ระบบเริ่มอ่านรถและป้ายทะเบียนได้

สิ่งที่ทำ:

- vehicle event service
- plate OCR / normalization
- known / unknown plate
- whitelist / blacklist / watchlist เบื้องต้น
- vehicle entry / exit logs

ผลลัพธ์:

- ประตูรถและลานจอดเริ่มใช้งานได้จริง

## Phase 6 — Hotel / Factory Business Rules

เป้าหมาย:

ต่อยอดไปยัง use case เฉพาะอุตสาหกรรม

สิ่งที่ทำ:

### Hotel

- guest-room mapping
- guest vehicle mapping
- guest floor rules
- visitor registration hooks

### Factory

- contractor access schedule
- vehicle gate rules
- warehouse zone rules
- after-hours factory alerts

ผลลัพธ์:

- ใช้กับโรงแรมและโรงงานได้เป็นระบบมากขึ้น

## Phase 7 — Behavior Expansion

เป้าหมาย:

เพิ่มความสามารถด้านพฤติกรรมที่ใช้ sequence/clip จริงจังมากขึ้น

สิ่งที่ทำ:

- fall detection
- loitering classification
- fence climbing
- suspicious carrying
- tailgating
- zone intrusion patterns

ผลลัพธ์:

- ระบบเริ่มตีความเหตุการณ์ซับซ้อนขึ้น

## Phase 8 — Model Lifecycle

เป้าหมาย:

รองรับการปรับ model โดยไม่เสียเสถียรภาพ

สิ่งที่ทำ:

- model-registry metadata
- active / previous model
- rollback policy
- gallery versioning
- fine-tune pipeline ในภายหลัง

ผลลัพธ์:

- เปลี่ยน model ได้ปลอดภัยขึ้น

## ลำดับที่แนะนำจริง

ถ้าจะทำให้ใช้งานได้เร็วที่สุด ให้เดินตามนี้:

1. Phase 0
2. Phase 1
3. Phase 2
4. Phase 3
5. Phase 4
6. Phase 5

ส่วน Phase 6–8 ค่อยทำเมื่อระบบหลักนิ่งแล้ว

## สิ่งที่ไม่ควรรีบทำ

- retrain behavior model ตั้งแต่วันแรก
- ทำ hotel/factory workflow เต็มระบบตั้งแต่ยังไม่มี MVP
- ทำ LPR หลายประเทศพร้อมกัน
- ทำ analytics dashboard ใหญ่ก่อนมี event schema คงที่
- ทำ role management ซับซ้อนก่อน registry เสถียร

## Milestone ที่แนะนำบน GitHub

### Milestone 1: Local Pilot

- repo bootstrap
- 1 camera
- person/vehicle detection
- zones + alerts

### Milestone 2: Structured Events

- normalized event schema
- registry foundation
- notifier

### Milestone 3: Identity Pilot

- known / unknown person
- attendance basics

### Milestone 4: Gate / Parking Pilot

- vehicle logs
- LPR basics
- watchlist

### Milestone 5: Vertical Presets

- home preset
- office preset
- hotel preset
- factory preset

## สรุป

roadmap นี้ตั้งใจให้ `home-guardian-ai`:

- เริ่มเล็ก
- ใช้งานได้เร็ว
- มีโครงที่ต่อยอดง่าย
- ไม่ยัดฟีเจอร์อนาคตเข้ามาพร้อมกันทั้งหมด
