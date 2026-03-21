# MVP-FIRST.md

## เป้าหมายของเอกสารนี้

เอกสารนี้ใช้กำหนดว่า `home-guardian-ai` ควรทำอะไร **ก่อน** เพื่อให้ใช้งานได้จริง และควรเลื่อนอะไรไปทีหลัง เพื่อไม่ให้เริ่มต้นยากเกินไป

## หลักการ

ให้เริ่มจากสิ่งที่:

- ติดตั้งและทดลองได้เร็ว
- ใช้กับกล้องจริงได้
- อธิบายผลลัพธ์ได้ง่าย
- ไม่ต้องใช้ dataset จำนวนมาก
- ไม่ต้อง retrain model ตั้งแต่วันแรก

## MVP ที่แนะนำ

### MVP v0.1 — Detection Pilot

ต้องทำให้ได้ก่อน:

- รับ RTSP จากกล้อง 1 ตัว
- รัน Frigate ได้ด้วย Docker Compose
- ตรวจจับ `person` ได้
- ตรวจจับ `vehicle` ได้
- บันทึก snapshot หรือ short clip ได้
- ดู event จาก UI หรือ log ได้

### MVP v0.2 — Rule-based Security Pilot

ต่อจาก v0.1:

- สร้าง zone เช่น `front-door`, `gate`, `parking`
- สร้าง line crossing สำหรับ `in` / `out`
- แจ้งเตือนเมื่อ:
  - unknown person at door
  - vehicle at gate
  - loitering near door
  - movement after hours
- เก็บ event log ในรูปแบบมาตรฐาน

### MVP v0.3 — Lightweight Registry Pilot

ต่อจาก v0.2:

- เพิ่มโครงสร้าง registry สำหรับคนและรถ
- เก็บ metadata เช่นชื่อ, ประเภท, allowed zone, schedule
- รองรับ known / unknown hooks
- เริ่มเตรียม schema สำหรับ attendance และ LPR

### MVP v0.4 — Identity / Attendance Pilot

ต่อจาก v0.3:

- face gallery สำหรับคนที่รู้จักจำนวนไม่มาก
- known person / unknown person
- เชื่อม line crossing กับ person identity
- บันทึก check-in / check-out ขั้นต้น

## อะไรควรเลื่อนไป phase ถัดไป

### ทำทีหลังแต่เตรียมโครงไว้ได้

- license plate OCR / LPR
- vehicle ownership mapping
- hotel guest-room mapping
- factory contractor rules
- blacklist / whitelist แบบเต็มระบบ
- fine-tuning model
- model rollback automation

### ยังไม่ควรเริ่มในรอบแรก

- multi-camera scale ใหญ่
- distributed service หลายตัวพร้อมกัน
- custom behavior training ตั้งแต่วันแรก
- dashboard analytics ขนาดใหญ่
- deep hotel PMS integration
- ERP / WMS / access control integration เต็มรูปแบบ

## ฟีเจอร์ที่ควรทำจริงก่อน แยกตามกลุ่ม

### Home

ทำก่อน:

- stranger detected
- resident returned home ในภายหลัง
- vehicle at gate
- unknown vehicle at gate ในภายหลัง
- loitering near door
- fence climbing ในภายหลัง
- fall detected ในภายหลัง

### Office

ทำก่อน:

- employee presence จาก line crossing
- access after hours
- unknown person in office zone
- vehicle entered staff parking

### Hotel

ทำก่อนแค่พื้นฐาน:

- person at lobby / guest-floor zone
- vehicle at hotel gate
- suspicious loitering near room corridor

เลื่อนไปทีหลัง:

- guest of room 1208 arrived
- unregistered visitor
- guest-room mapping เต็มรูปแบบ

### Factory

ทำก่อน:

- person in restricted zone
- vehicle at loading gate
- after-hours warehouse movement

เลื่อนไปทีหลัง:

- PPE violation
- contractor zone schedule
- forklift/person proximity
- delivery schedule validation

## สรุปฟีเจอร์ที่ควรอยู่ใน milestone แรก

### Must-have

- RTSP ingest
- person detection
- vehicle detection
- snapshot/event clip
- line crossing
- zone rules
- basic alerts
- event log schema

### Nice-to-have

- simple person registry
- simple vehicle registry
- basic known/unknown logic hook

### Later

- face recognition
- LPR
- hotel guest mapping
- factory workflow rules
- advanced behavior model
- retrain + rollback

## Acceptance criteria

Milestone แรกถือว่าใช้ได้ เมื่อ:

1. เปิด stack ได้ด้วยคำสั่งเดียว
2. เชื่อมกล้อง 1 ตัวได้
3. ตรวจจับคนและรถได้จริง
4. บันทึก snapshot/event ได้
5. สร้าง rule แจ้งเตือนจาก zone/line crossing ได้
6. มีเอกสารอธิบายวิธีเริ่มต้นครบ

## แนวคิดสำคัญ

ระบบนี้ควรโตแบบนี้:

- เริ่มจาก detection และ rules
- เพิ่ม registry
- ค่อยเพิ่ม identity
- ค่อยเพิ่ม attendance ที่อิง identity
- ค่อยเพิ่ม LPR และ guest mapping
- ค่อยเพิ่ม retraining และ rollback

ถ้ารักษาลำดับนี้ได้ โปรเจกต์จะไม่เริ่มต้นยากเกินไป และจะเพิ่มฟีเจอร์ภายหลังได้ง่ายกว่า
