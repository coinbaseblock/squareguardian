# Square Guardian AI

Local AI video intelligence for home, office, hotel, and factory: person identity, behavior analysis, attendance, vehicle and license plate recognition, guest/room mapping, alerting, and model rollback.

## แนวคิดหลัก

โปรเจกต์นี้ตั้งใจทำระบบกล้อง AI แบบ **ทำให้ใช้งานได้จริงก่อน** แล้วค่อยเพิ่มความสามารถภายหลัง โดยแยกชัดว่าอะไรคือ:

- **MVP ที่ควรทำก่อน** เพื่อให้มีของใช้งานเร็ว
- **Phase ถัดไป** เพื่อเพิ่มความแม่นยำและความสามารถ
- **Future scope** ที่ยังไม่ควรรีบทำในรอบแรก

เป้าหมายคือหลีกเลี่ยงการเขียนเอกสารใหญ่เกินจริง แต่ลงมือทำจริงได้แค่บางส่วน จนทำให้ repo ซับซ้อนเกินความจำเป็น

## เป้าหมายของรอบแรก

รอบแรกให้ระบบทำงานได้กับงานที่จำเป็นที่สุดก่อน:

1. รับภาพจากกล้อง RTSP ได้
2. ตรวจจับ `person` และ `vehicle` ได้
3. บันทึก snapshot / event clip ได้
4. กำหนด zone / line crossing ได้
5. แจ้งเตือนเหตุการณ์สำคัญได้
6. มีโครงสร้าง registry สำหรับคนและรถ เพื่อให้เพิ่ม identity และ LPR ทีหลังได้ง่าย

## สิ่งที่ควรทำจริงก่อน

### MVP v0.1 — Smoke Test

เป้าหมายคือให้ pipeline ติดก่อน

- Docker Compose รันได้
- Frigate รันได้
- กล้อง 1 ตัวเชื่อม RTSP ได้
- ตรวจจับ `person` ได้
- ตรวจจับ `vehicle` ได้
- มี snapshot และ event ใน UI

### MVP v0.2 — ใช้งานจริงระดับพื้นฐาน

เป้าหมายคือเริ่มใช้เฝ้าระวังจริงได้

- เพิ่ม zone เช่น ประตูหน้า, รั้ว, ลานจอดรถ
- เพิ่ม line crossing สำหรับเข้า/ออก
- แจ้งเตือน `unknown person`, `vehicle at gate`, `loitering` แบบ rule-based
- บันทึก event log พร้อมเวลา กล้อง และโซน
- เก็บ registry ของคนและรถแบบ JSON/YAML

### MVP v0.3 — เริ่มมี identity และ attendance

- face gallery สำหรับคนที่รู้จักจำนวนไม่มาก
- known / unknown person
- line crossing + identity = check-in / check-out ขั้นต้น
- known vehicle / unknown vehicle จากทะเบียนหรือ watchlist แบบง่าย

## สิ่งที่ยังไม่ควรทำในรอบแรก

- multi-camera scale ใหญ่
- full retraining pipeline ตั้งแต่วันแรก
- hotel room mapping แบบสมบูรณ์
- ERP/WMS integration โรงงาน
- full workflow ของ blacklist / whitelist หลายระดับ
- advanced action recognition จำนวนมาก
- model rollback UI เต็มรูปแบบ

## กลุ่มความสามารถหลัก

### 1. Detection
ใช้สำหรับตอบว่าในภาพมีอะไรอยู่บ้าง

- person
- vehicle
- face
- bag
- motorcycle
- truck

### 2. Identity
ใช้สำหรับตอบว่าเป็นใคร

- resident
- staff
- guest
- contractor
- unknown

### 3. Behavior
ใช้สำหรับตอบว่ากำลังทำอะไร

- เดินผ่าน
- ยืนแช่
- เดินวน
- ปีนรั้ว
- ล้ม
- ขนของ
- เข้าเขตหวงห้าม

### 4. Attendance / Presence
ใช้สำหรับตอบว่าเข้าออกเมื่อไร

- check-in
- check-out
- late
- early leave
- after-hours presence

### 5. Vehicle / LPR
ใช้สำหรับตอบว่ารถอะไร ป้ายอะไร ของใคร

- vehicle detection
- plate OCR
- known vehicle / unknown vehicle
- watchlist / blacklist / whitelist
- entry / exit log

### 6. Access / Guest Mapping
ใช้สำหรับตอบว่าใครหรือรถนี้มีสิทธิ์อะไร

- แขกห้องอะไร
- รถของห้องไหน
- คนนี้เข้าโซนไหนได้
- รถ supplier เข้าช่องไหนได้

## หลักการออกแบบ

### ทำของที่ใช้ได้ก่อน

ถ้าฟีเจอร์ไหนต้องใช้ dataset เยอะ, retrain ยุ่ง, หรือมี dependency เยอะ ให้เลื่อนไป phase ถัดไปก่อน

### แยกชั้นให้ชัด

- Detection = เจออะไร
- Identity = ใคร
- Behavior = ทำอะไร
- Presence = เข้าออกเมื่อไร
- Vehicle/LPR = รถอะไร ทะเบียนอะไร
- Access Mapping = มีสิทธิ์อะไร

### อย่า overfit กับ use case เดียว

แม้ชื่อจะเป็น `home-guardian-ai` แต่โครงควรรองรับ:

- บ้าน
- ออฟฟิศ
- โรงแรม
- โรงงาน
- ลานจอดรถ / ประตูรั้ว

### อย่าผูกกับ model เดียว

โครงสร้างข้อมูลและ service ควรออกแบบให้เปลี่ยน model ได้ในภายหลัง เช่น:

- face model
- behavior model
- lpr model
- detector model

## สภาพแวดล้อมที่แนะนำ

### ระยะเริ่มต้น

- Windows 11 + Docker Desktop + WSL2
- เก็บ project ใน WSL filesystem
- ใช้ Frigate แบบ config เบา ๆ ก่อน
- ยังไม่บังคับ hardware acceleration ตั้งแต่วันแรก

### ระยะใช้งานจริงจัง

- Ubuntu / Debian bare metal
- OpenVINO / Intel QSV / NPU / GPU ตามความพร้อม
- เปิดบริการเสริมเช่น face recognition, LPR, behavior model ทีละชั้น

## โครงสร้าง repo ที่แนะนำ

```text
home-guardian-ai/
  AGENTS.md
  README.md
  docs/
    ARCHITECTURE.md
    MVP-FIRST.md
    ROADMAP.md
    FEATURE_MATRIX.md
    DATA_MODELS.md
```

เมื่อลงมือทำจริงค่อยขยายเพิ่มเป็น:

```text
home-guardian-ai/
  services/
    nvr/
    identity/
    behavior/
    attendance/
    vehicle/
    lpr/
    guest-mapping/
    access-control/
    notifier/
    registry/
    model-registry/
  data/
  docs/
  scripts/
  config/
```

## ใช้กับสถานที่ประเภทไหนได้บ้าง

### บ้าน

- คนแปลกหน้า
- คนในบ้านกลับถึงบ้าน
- รถแปลกหน้าเข้าประตู
- ล้ม
- เดินวนหน้าประตู
- ปีนรั้ว

### สำนักงาน

- พนักงานเข้างาน/ออกงาน
- อยู่ในพื้นที่หลังเวลางาน
- คนไม่รู้จักเข้าพื้นที่พนักงาน
- รถเข้าลานจอดพนักงาน
- ขนของออกนอกพื้นที่

### โรงแรม

- แขกกลับเข้าที่พัก
- คนไม่รู้จักอยู่ชั้นห้องพัก
- รถแขกมาถึง
- แขกหรือ visitor ไม่ลงทะเบียน
- เดินวนหน้าห้อง

### โรงงาน

- พนักงาน / ผู้รับเหมาเข้าออก
- รถบรรทุกเข้าออก
- รถเข้า zone อันตราย
- PPE violation ในอนาคต
- คนอยู่ในคลังหลังเวลางาน

## เอกสารในชุดนี้

- `README.md` — ภาพรวมและแนวทางการเริ่มต้น
- `AGENTS.md` — กติกาสำหรับ Codex / Claude Code / AI coding agent
- `docs/MVP-FIRST.md` — ขอบเขตที่ควรทำก่อนจริง ๆ
- `docs/ARCHITECTURE.md` — สถาปัตยกรรมและการแยกชั้น
- `docs/ROADMAP.md` — ลำดับการพัฒนา
- `docs/FEATURE_MATRIX.md` — อะไรทำก่อน อะไรค่อยเพิ่ม
- `docs/DATA_MODELS.md` — schema และตัวอย่างไฟล์ข้อมูล

## สรุปสั้น

`home-guardian-ai` ควรเริ่มจากระบบที่ทำได้จริงก่อน:

- RTSP
- person detection
- vehicle detection
- zones / line crossing
- alerts
- logs
- simple registry

แล้วค่อยต่อยอดเป็น:

- face identity
- attendance
- LPR
- guest mapping
- hotel/factory presets
- retrain / rollback
