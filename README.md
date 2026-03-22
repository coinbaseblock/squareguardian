# SpaceGuardian

SpaceGuardian คือชุดเริ่มต้นสำหรับทำระบบกล้อง AI แบบ **เปิดง่าย ใช้งานง่ายก่อน** โดยโฟกัสที่ local pilot 1 กล้องผ่าน RTSP เหมาะกับบ้าน หน้าร้าน ออฟฟิศ หรือประตูเข้าออกเล็ก ๆ.

รอบนี้เตรียม preset แบบง่ายสำหรับกล้อง **TP-Link Tapo** ผ่าน Frigate + Docker Compose โดยไม่ commit credential กล้องลง repo.

## สิ่งที่ได้ใน MVP ชุดนี้

- รันด้วย `docker compose up -d`
- ใช้กล้อง RTSP 1 ตัว
- ตรวจจับ `person` และ `vehicle` (`car`, `motorcycle`, `bus`, `truck`)
- มี snapshot เก็บไว้ดูย้อนหลัง
- มีตัวอย่าง `zone` เริ่มต้นชื่อ `front_door`
- เปิดดูผ่าน Frigate UI ได้ทันที
- มีสคริปต์เริ่มระบบและล้างระบบสำหรับ Windows/CMD โดยไม่ต้องพิมพ์ `prune` เองทุกครั้ง

> หมายเหตุ: ชุดนี้ตั้งใจให้เริ่มใช้งานได้เร็วที่สุดก่อน จึงยัง **ไม่ใส่ identity, attendance, LPR, multi-camera orchestration** ในรอบแรก

## โครงสร้างที่เพิ่มเข้ามา

```text
spaceguardian/
  docker-compose.yml
  .env.example
  config/
    frigate/
      config.yml
  scripts/
    dev-up.ps1
    dev-up.cmd
    clean.ps1
    clean.cmd
  storage/
    frigate/
  docs/
```

## Quick start สำหรับ TP-Link Tapo

### 1) สร้างไฟล์ `.env`

```bash
cp .env.example .env
```

จากนั้นแก้ `.env` ให้เป็น RTSP จริงของคุณ เช่น

```env
FRIGATE_TAPO_RTSP_URL=rtsp://username:password@192.168.1.50/stream1
FRIGATE_TIMEZONE=Asia/Bangkok
```

ถ้าคุณใช้ Tapo main stream ส่วนใหญ่จะเป็น path ประมาณ `/stream1`.

> เพื่อความปลอดภัย อย่า commit `.env` เพราะไฟล์นี้เก็บ credential กล้องจริง

### 2) เปิดระบบ

#### Windows CMD

```cmd
scripts\dev-up.cmd
```

#### PowerShell

```powershell
./scripts/dev-up.ps1
```

สคริปต์นี้จะทำให้ครบตาม flow ที่คุณทำมืออยู่ตอนนี้:

1. สร้าง `.env` จาก `.env.example` อัตโนมัติถ้ายังไม่มี
2. export binary ไปที่ `dist/`
3. รัน `docker compose up -d --build`
4. แสดง URL และสถานะ container ตอนท้าย

ถ้าต้องการ rebuild แบบล้าง cache project ก่อน:

```cmd
scripts\dev-up.cmd -Clean
```

ถ้าต้องการบังคับ build แบบไม่ใช้ layer cache ของ compose เพิ่มอีกชั้น:

```cmd
scripts\dev-up.cmd -Clean -NoCache
```

### 3) เปิดหน้าเว็บ

- Frigate UI: `http://localhost:8971`
- Detector API: `http://localhost:8080`
- API/config debug: `http://localhost:5000`

### 4) ตรวจสอบว่ากล้องขึ้น

```bash
docker compose logs -f frigate
```

ถ้าสตรีมมาปกติ ให้เข้า UI แล้วดูว่ามีภาพสดและเริ่มมี event / snapshot จาก `person` หรือ `vehicle`

## ลบ/เก็บกวาดแบบไม่ต้อง manual

ถ้าต้องการหยุด stack และล้างไฟล์ runtime ของ project:

```cmd
scripts\clean.cmd
```

แนะนำให้เรียกผ่าน `clean.cmd` แทน `clean.ps1` โดยตรง เพราะตัว `.cmd` จะตั้ง `-ExecutionPolicy Bypass` ให้ในรอบนั้น และช่วยเลี่ยงปัญหา PowerShell policy บน Windows เครื่องใหม่ ๆ ได้

สคริปต์นี้จะ:

- `docker compose down --remove-orphans --volumes`
- ล้างไฟล์ใน `storage/events`
- ล้างไฟล์ใน `storage/frigate` แต่เก็บ `.gitkeep`
- ลบโฟลเดอร์ `dist/`

ถ้าต้องการล้าง Docker cache/image/volume ที่ไม่ใช้งานเพิ่มด้วยแบบเดียวกับที่คุณพิมพ์เอง:

```cmd
scripts\clean.cmd -All
```

คำสั่งนี้จะรวม `docker builder prune -af` และ `docker system prune -af --volumes` ด้วย ดังนั้นควรใช้ตอนที่ต้องการเคลียร์เครื่องจริง ๆ

## ถ้าอยากทำแบบ manual เหมือนเดิม

ยังทำได้ตามนี้:

```bash
docker build --target export -o dist .
docker compose up -d --build
```

ถ้าต้องการล้างทุกอย่างก่อน build:

```bash
docker compose down --remove-orphans --volumes
docker builder prune -af
docker system prune -af --volumes
docker build --target export -o dist .
docker compose up -d --build
```

## ค่าเริ่มต้นที่ตั้งไว้ให้แล้ว

ไฟล์ `config/frigate/config.yml` ถูกตั้งให้เรียบง่ายก่อน:

- ปิด MQTT เพื่อให้เริ่มแบบ standalone ได้
- ปิด continuous recording เพื่อลด storage
- เปิด snapshots ไว้ 7 วัน
- track เฉพาะ `person` และกลุ่ม `vehicle`
- มี zone ตัวอย่าง `front_door`
- ใช้ CPU detector เป็นค่าเริ่มต้นเพื่อให้เริ่มได้แม้ยังไม่มี accelerator

## หมายเหตุเรื่องการ build Docker

ตอนนี้ service `squareguardian` ใช้เฉพาะ Go standard library จึงอาจยังไม่มีไฟล์ `go.sum` ใน repo ได้ตามปกติ. Docker build ถูกตั้งให้ใช้ `go.mod` ได้โดยตรง และใช้ Go 1.25 ให้ตรงกับเวอร์ชันที่ระบุใน module.

สำหรับการใช้งานแบบ local/offline ใน repo นี้ `go.mod` ถูกตั้ง module เป็น `squareguardian` ตรง ๆ แล้ว จึงไม่ต้องอ้าง path แบบ remote เช่น `github.com/coinbaseblock/squareguardian`.

ถ้าคุณเห็นว่ารอบที่สองยัง download image layer เยอะอยู่บน Windows + Docker Desktop สาเหตุหลักมักเป็น cache ฝั่ง Docker/WSL ยังไม่อุ่น, มีการ prune มาก่อน, หรือมีการ rebuild ด้วย context ใหม่. ถ้าไม่ได้แก้ Dockerfile/go.mod บ่อย ๆ รอบถัดไปควรเร็วขึ้นเมื่อไม่ใช้ `-Clean` หรือ `-All`.

## ถ้าจะปรับให้เหมาะกับจุดติดตั้งจริง

แนะนำให้ปรับ 3 อย่างก่อน:

1. `zones.front_door.coordinates` ให้ตรงกับพื้นที่หน้าประตูจริง
2. `detect.width` / `detect.height` ให้ตรงกับ stream ที่ใช้
3. `motion.mask` เพื่อตัด timestamp หรือพื้นที่ที่ขยับตลอดเวลาออก

## แนวทาง MVP ของ SpaceGuardian

SpaceGuardian ควรโตตามลำดับนี้:

1. RTSP + person/vehicle detection
2. zone / rule / alert แบบง่าย
3. event log + registry
4. identity / attendance
5. vehicle + lpr
6. access-control / guest-mapping

หลักคือ **ทำของที่ใช้งานได้จริงก่อน** แล้วค่อยขยาย layer ที่ซับซ้อนขึ้น

## เอกสารที่เกี่ยวข้อง

- `docs/MVP-FIRST.md` — MVP ที่ควรทำก่อนจริง ๆ
- `docs/ARCHITECTURE.md` — โครงระบบแบบแยก layer
- `docs/ROADMAP.md` — ลำดับการขยายระบบ
- `docs/DATA_MODELS.md` — schema registry / event ที่เผื่อโตต่อ
- `docs/FEATURE_MATRIX.md` — what to do now / next / later

## ข้อเสนอแนะถัดไป

ถ้าชุดนี้รันผ่านและภาพ Tapo ขึ้นแล้ว งานถัดไปที่ควรทำมี 3 อย่าง:

1. เพิ่ม `parking` และ `gate` zones สำหรับรถ
2. เพิ่ม notifier ง่าย ๆ เช่น LINE / Telegram จาก event ที่เข้า zone
3. เพิ่ม event logger แบบไฟล์ JSON เพื่อเก็บประวัติใช้งานจริง
