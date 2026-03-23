# MVP-FIRST.md

## เป้าหมาย

เอกสารนี้กำหนดว่า `SquareGuardian` ควรทำอะไร **ก่อน** เพื่อให้เริ่มใช้งานกับกล้องจริงได้เร็วที่สุด โดยยังเปิดทางให้ขยายไป identity, attendance, vehicle, lpr และ access-control ภายหลังได้.

## MVP ที่ควรโฟกัสตอนนี้

### MVP v0.1 — Single Camera Pilot

ต้องทำให้ได้ก่อน:

- Docker Compose สตาร์ตได้ด้วยคำสั่งเดียว
- มี script สำหรับ start/cleanup บนเครื่อง dev โดยไม่ต้องจำชุดคำสั่ง prune เอง
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
2. มีทางเลือกแบบ script สำหรับ start/export/cleanup บน Windows ได้ทันที
3. Tapo RTSP 1 ตัวเชื่อมได้จริง
4. เห็น `person` และ `vehicle` ใน Frigate UI
5. มี snapshot จาก event
6. ปรับ zone เองได้จาก config
7. README อธิบายการเริ่มต้นและการล้างระบบได้ครบ

หมายเหตุการใช้งาน UI:

- ให้ใช้ Frigate UI ผ่านพอร์ต `8971`
- ถ้าต้อง debug API ให้ใช้พอร์ต `5001`
- ถ้าต้อง debug live stream ให้ใช้ go2rtc ผ่านพอร์ต `1984`
- สำหรับ browser live view ให้แยก stream transcode เป็น H.264 ไว้ต่างหาก ถ้ากล้องปล่อย H.265/HEVC แล้วภาพขึ้นไม่กี่วินาทีค่อยดับ
- ไม่ควรเปิดหน้า Explore ผ่าน `5000` เพราะ thumbnail/snapshot อาจไม่แสดงในบาง environment
- RTSP URL ควรถูกส่งเข้าคอนเทนเนอร์ผ่าน env ที่ Frigate substitute ได้จริง (เช่น `FRIGATE_CAMERA_FRONT_RTSP_URL`)

## แนวคิดสำคัญ

- เริ่มจาก detection ก่อน
- ใช้ zone/rule ที่เข้าใจง่ายก่อน
- อย่าเพิ่ม dependency หนักถ้ายังไม่จำเป็น
- เก็บ credential ผ่าน `.env` เท่านั้น
- ใช้ `CAMERA_FRONT_RTSP_URL` เป็นชื่อตัวแปรเดียวสำหรับ RTSP URL (รองรับกล้องทุกยี่ห้อ)
- documentation ต้องพาคนเริ่มได้จริง
- cleanup ต้องทำซ้ำได้ง่ายและไม่บังคับให้จำ manual steps
- start/cleanup script ควรหา root ของโปรเจกต์เองได้ เพื่อให้เรียกจาก root repo หรือโฟลเดอร์ `scripts/` บน Windows ได้เหมือนกัน
