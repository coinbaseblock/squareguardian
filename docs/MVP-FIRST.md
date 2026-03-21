# MVP-FIRST.md

## เป้าหมาย

เอกสารนี้กำหนดว่า `SpaceGuardian` ควรทำอะไร **ก่อน** เพื่อให้เริ่มใช้งานกับกล้องจริงได้เร็วที่สุด โดยยังเปิดทางให้ขยายไป identity, attendance, vehicle, lpr และ access-control ภายหลังได้.

## MVP ที่ควรโฟกัสตอนนี้

### MVP v0.1 — Single Camera Pilot

ต้องทำให้ได้ก่อน:

- Docker Compose สตาร์ตได้ด้วยคำสั่งเดียว
- กล้อง RTSP 1 ตัวใช้งานได้
- ตรวจจับ `person` ได้
- ตรวจจับ `vehicle` ได้
- มี snapshot และ event ใน UI
- มี zone เบื้องต้นอย่างน้อย 1 จุด

### MVP v0.2 — Simple Security Rules

ต่อจาก v0.1:

- เพิ่ม zones เช่น `front-door`, `gate`, `parking`
- เพิ่มกติกาแจ้งเตือนจาก zone
- เก็บ event log ในรูปแบบมาตรฐาน
- ลด false positive ด้วย motion mask และ threshold

### MVP v0.3 — Registry Foundation

ต่อจาก v0.2:

- person registry แบบ JSON/YAML
- vehicle registry แบบ JSON/YAML
- known / unknown hooks แบบเรียบง่าย
- event normalization สำหรับการต่อ notifier หรือ dashboard

## สิ่งที่ยังไม่ต้องรีบทำ

ยังไม่ควรยัดเข้ารอบแรก:

- face recognition เต็ม flow
- attendance เต็มระบบ
- LPR / plate OCR
- multi-camera orchestration
- analytics dashboard ขนาดใหญ่
- hotel/factory integration เต็มรูปแบบ

## Acceptance criteria ของ milestone แรก

ถือว่า MVP ใช้งานได้ เมื่อ:

1. `docker compose up -d` แล้ว stack ขึ้น
2. Tapo RTSP 1 ตัวเชื่อมได้จริง
3. เห็น `person` และ `vehicle` ใน Frigate UI
4. มี snapshot จาก event
5. ปรับ zone เองได้จาก config
6. README อธิบายการเริ่มต้นได้ครบ

## แนวคิดสำคัญ

- เริ่มจาก detection ก่อน
- ใช้ zone/rule ที่เข้าใจง่ายก่อน
- อย่าเพิ่ม dependency หนักถ้ายังไม่จำเป็น
- เก็บ credential ผ่าน `.env` เท่านั้น
- documentation ต้องพาคนเริ่มได้จริง
