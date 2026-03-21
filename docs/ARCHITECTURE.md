# ARCHITECTURE.md

## ภาพรวม

`home-guardian-ai` ควรออกแบบให้แยกชั้นตั้งแต่ต้น เพื่อให้เริ่มจาก MVP ง่าย ๆ ได้ และเพิ่มฟีเจอร์ทีหลังโดยไม่ต้องรื้อระบบทั้งหมด

## ชั้นของระบบ

### 1. Detection Layer

หน้าที่:

- รับภาพจากกล้อง
- detect person / vehicle / face / bag
- ส่ง event พื้นฐานออกมา

ตัวเลือกเริ่มต้น:

- Frigate
- OpenVINO ในภายหลัง

สถานะ:

- **ควรทำจริงทันที**

### 2. Rule / Event Layer

หน้าที่:

- รับ event จาก detector
- ตีความ zone / line crossing
- สร้าง alert พื้นฐาน
- normalize event ให้เป็น schema เดียวกัน

ตัวอย่าง rule:

- person entered front-door zone
- vehicle crossed gate-in line
- movement after hours
- loitering > threshold

สถานะ:

- **ควรทำจริงทันที**

### 3. Registry Layer

หน้าที่:

- เก็บข้อมูลคน รถ และ metadata
- ใช้สำหรับ known / unknown logic
- เก็บ allowed zones และ schedules

ตัวอย่างข้อมูล:

- staff profile
- resident profile
- guest profile
- vehicle profile

สถานะ:

- **ควรทำจริงแบบ lightweight ตั้งแต่ต้น**

### 4. Identity Layer

หน้าที่:

- บอกว่า person นี้คือใคร
- known / unknown
- เชื่อมกับ attendance ในภายหลัง

ตัวเลือกเริ่มต้น:

- face embedding gallery
- person re-id ในภายหลัง

สถานะ:

- **phase ถัดไป**

### 5. Attendance / Presence Layer

หน้าที่:

- check-in / check-out
- late / early leave
- presence duration
- after-hours detection

อินพุตหลัก:

- identity + line crossing + timestamp

สถานะ:

- **เริ่มด้วย rule-based presence ก่อน**
- **full identity attendance ค่อยทำทีหลัง**

### 6. Vehicle / LPR Layer

หน้าที่:

- vehicle entry / exit
- plate OCR
- known / unknown plate
- watchlist / blacklist / whitelist

สถานะ:

- vehicle tracking = **เริ่มโครงได้เร็ว**
- LPR = **phase ถัดไป**

### 7. Access / Guest Mapping Layer

หน้าที่:

- ผูกบุคคลหรือรถกับห้อง สิทธิ์ หรือ schedule
- ใช้กับโรงแรม โรงงาน และ access control use case

สถานะ:

- **เก็บ schema ไว้ก่อน**
- **ยังไม่ควรทำเต็มระบบใน milestone แรก**

### 8. Model Registry Layer

หน้าที่:

- เลือก active model
- เก็บ version metadata
- rollback ไป version ก่อนหน้าได้

สถานะ:

- **phase หลังจากมี model มากกว่า 1 ตัวจริง ๆ**

## สถาปัตยกรรมที่ควรเริ่มจริงก่อน

```text
RTSP Camera
  -> Frigate
  -> Detection Event
  -> Rule Engine
  -> Event Logger
  -> Notifier
```

โครงแบบนี้เพียงพอสำหรับ milestone แรก

### สิ่งที่ควรมีในรอบแรก

- RTSP ingest
- detector
- zone / line crossing
- alert rules
- event logging
- notifier

### สิ่งที่ยังไม่ต้องยัดเข้ารอบแรก

- face model หลายรุ่น
- behavior model หลายตัว
- LPR service แยกหลายตัว
- hotel/factory business integration

## สถาปัตยกรรมที่ควรโตใน phase ถัดไป

```text
RTSP Camera
  -> Frigate / Detector
  -> Event Normalizer
  -> Rule Engine
  -> Registry Lookup
  -> Identity Enrichment
  -> Vehicle / LPR Enrichment
  -> Attendance / Access Logic
  -> Event Store
  -> Notifier / Dashboard
```

## หลักการแยก service

### nvr
รับภาพและ event ขั้นต้น

### registry
เก็บข้อมูลคน รถ และ metadata

### notifier
ส่งแจ้งเตือน LINE / Telegram / Email

### identity
แยก known / unknown person

### attendance
สรุป check-in / check-out และ duration

### vehicle
เก็บ vehicle event และ vehicle matching

### lpr
ทำ OCR ป้ายทะเบียน

### guest-mapping
ผูกกับห้องพัก ผู้เข้าพัก หรือ visitor

### access-control
ตัดสิน allowed / denied / after-hours / restricted zone

### model-registry
เก็บ active model, previous model, rollback metadata

## แนวทางแยกตาม phase

### Phase 1 — ใช้งานก่อน

ใช้จริงแค่:

- `nvr`
- `notifier`
- rule logic แบบง่าย
- event logger

### Phase 2 — ขยายอย่างปลอดภัย

เพิ่ม:

- `registry`
- `identity`
- `attendance`

### Phase 3 — รถและป้ายทะเบียน

เพิ่ม:

- `vehicle`
- `lpr`
- watchlist / whitelist / blacklist

### Phase 4 — use case เฉพาะธุรกิจ

เพิ่ม:

- `guest-mapping`
- `access-control`
- hotel / factory presets

### Phase 5 — model lifecycle

เพิ่ม:

- `model-registry`
- retraining pipeline
- rollback policy

## Data flow ที่แนะนำ

### Event flow พื้นฐาน

1. กล้องส่ง RTSP
2. Detector ตรวจเจอ object
3. ระบบแปลงเป็น normalized event
4. Rule engine ประเมิน zone / line / schedule
5. เก็บ log
6. แจ้งเตือนถ้าตรงเงื่อนไข

### Event flow เมื่อมี identity

1. detector เจอ person
2. identity service พยายาม match กับ gallery
3. ถ้า match ได้ -> known person
4. ถ้า match ไม่ได้ -> unknown
5. attendance/rules ใช้ข้อมูลนี้ต่อ

### Event flow เมื่อมี LPR

1. detector เจอ vehicle
2. crop plate region
3. lpr ทำ OCR
4. normalize plate
5. lookup จาก vehicle registry
6. ส่งผลเป็น known / unknown / blacklisted

## หลักการสำคัญของ architecture นี้

- MVP ควรอยู่ได้โดยไม่มี identity และ LPR ก่อน
- การเพิ่ม identity และ LPR ต้องไม่ทำให้ detection พัง
- Registry ควรแยกจาก model
- Attendance ควรเป็นผลจาก rules + identity ไม่ใช่ behavior model ตรง ๆ
- Guest mapping และ hotel/factory logic ควรเป็นชั้น business rule ไม่ใช่ยัดไว้ใน detector

## สรุป

สถาปัตยกรรมที่ดีของ `home-guardian-ai` คือเริ่มเล็ก แต่รองรับการโต:

- detection ก่อน
- rules ต่อ
- registry ตาม
- identity / attendance ทีหลัง
- vehicle / LPR ตามมา
- guest mapping / factory access logic ทีหลัง
- retraining / rollback ตอนมี model lifecycle จริง
