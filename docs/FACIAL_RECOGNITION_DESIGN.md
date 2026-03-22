# Local Facial Recognition Design

## ภาพรวม

SpaceGuardian จะมีระบบ Facial Recognition แบบ **Local-first** สองแนวทางที่ทำงานร่วมกัน:

1. **SpaceGuardian FR** — ระบบจดจำใบหน้าของเราเอง (InsightFace/FaceNet) รันบน server
2. **Tapo FR Bridge** — ดึงผลจดจำใบหน้าจากกล้อง Tapo ที่มี AI ในตัว (C225, C325WB ฯลฯ)

ทั้งสองระบบจะ feed เข้า **Unified Identity Registry** เดียวกัน ทำให้ผู้ใช้ได้ประโยชน์จากทั้งสองฝั่ง

---

## สถาปัตยกรรม

```text
                    ┌─────────────────────────────────────┐
                    │        Unified Identity Registry     │
                    │   (embedding store + person profile) │
                    └──────────┬──────────┬───────────────┘
                               │          │
              ┌────────────────▼──┐   ┌───▼─────────────────┐
              │  SpaceGuardian FR │   │   Tapo FR Bridge     │
              │  (Self-hosted)    │   │   (Camera-native)    │
              │                   │   │                      │
              │  InsightFace /    │   │  pytapo / ONVIF      │
              │  FaceNet          │   │  event polling       │
              │  face-api         │   │                      │
              └────────┬─────────┘   └───────┬──────────────┘
                       │                     │
              ┌────────▼─────────┐   ┌───────▼──────────────┐
              │  RTSP Stream     │   │  Tapo Camera         │
              │  via Frigate     │   │  (C225/C325WB)       │
              │  (snapshot/crop) │   │  Built-in AI chip    │
              └──────────────────┘   └──────────────────────┘
```

---

## Part 1: SpaceGuardian FR (ระบบของเราเอง)

### ทำไมต้องมีของตัวเอง

- ใช้ได้กับ **ทุกกล้อง** ที่ส่ง RTSP ได้ (ไม่จำกัดแค่ Tapo)
- ควบคุม model, threshold, และ embedding ได้เต็มที่
- รองรับ continuous learning จาก grouped events ที่มีอยู่แล้ว
- ข้อมูลไม่ออกนอก network

### Tech Stack

| Component | เลือกใช้ | เหตุผล |
|-----------|---------|--------|
| Face Detection | SCRFD / RetinaFace | เร็ว แม่นยำ ทำงานบน CPU ได้ |
| Face Recognition | InsightFace (ArcFace) | State-of-the-art, มี pretrained model ขนาดเล็ก |
| Runtime | ONNX Runtime | รองรับทั้ง CPU/GPU, cross-platform |
| Service | Python (FastAPI) | ecosystem สำหรับ ML ดีที่สุด |
| Embedding Store | SQLite + numpy | เบา ไม่ต้องพึ่ง vector DB ในช่วง MVP |

### Pipeline

```text
Frigate Event (person detected)
  │
  ▼
1. Crop face จาก snapshot
   ├── ใช้ SCRFD detect face ในภาพ
   ├── align face (5-point landmark)
   └── crop 112x112 px
  │
  ▼
2. Extract embedding (512-dim vector)
   └── ArcFace model (buffalo_s — ขนาด ~30MB)
  │
  ▼
3. Compare กับ Known Embeddings
   ├── cosine similarity
   ├── > 0.45 → match → auto-assign identity
   ├── > 0.35 → suggest → แจ้งผู้ใช้ confirm
   └── < 0.35 → unknown → เก็บเป็น new face
  │
  ▼
4. Update Event + Notify
   └── POST /api/annotate { identity: "สมชาย" }
```

### Docker Service ใหม่: `face-service`

```yaml
# เพิ่มใน docker-compose.yml
face-service:
  build:
    context: ./services/face
    dockerfile: Dockerfile
  ports:
    - "8082:8082"
  volumes:
    - ./storage/face-db:/data
  environment:
    - MODEL_NAME=buffalo_s          # เล็ก เร็ว พอสำหรับ MVP
    - MATCH_THRESHOLD=0.45
    - SUGGEST_THRESHOLD=0.35
    - MAX_FACES_PER_FRAME=10
  restart: unless-stopped
```

### API Endpoints ของ face-service

```
POST /api/face/detect
  Body: { "image": "<base64>" }
  Response: { "faces": [{ "bbox": [...], "embedding": [...], "confidence": 0.98 }] }

POST /api/face/identify
  Body: { "image": "<base64>" }
  Response: { "matches": [{ "person_id": "p-001", "name": "สมชาย", "similarity": 0.87 }] }

POST /api/face/register
  Body: { "person_id": "p-001", "name": "สมชาย", "images": ["<base64>", ...] }
  Response: { "status": "registered", "embeddings_count": 5 }

GET /api/face/gallery
  Response: { "persons": [{ "id": "p-001", "name": "สมชาย", "face_count": 5, "created_at": "..." }] }

DELETE /api/face/gallery/{person_id}
  Response: { "status": "deleted" }

POST /api/face/compare
  Body: { "image1": "<base64>", "image2": "<base64>" }
  Response: { "similarity": 0.92, "same_person": true }
```

### Embedding Storage Schema (SQLite)

```sql
CREATE TABLE persons (
    id          TEXT PRIMARY KEY,    -- "p-001"
    name        TEXT NOT NULL,
    source      TEXT DEFAULT 'manual', -- 'manual' | 'tapo' | 'auto'
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE face_embeddings (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    person_id   TEXT REFERENCES persons(id),
    embedding   BLOB NOT NULL,       -- 512-dim float32 = 2048 bytes
    source      TEXT DEFAULT 'manual', -- 'manual' | 'frigate' | 'tapo'
    quality     REAL,                -- face quality score 0-1
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE face_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id    TEXT NOT NULL,        -- frigate event id
    person_id   TEXT REFERENCES persons(id),
    similarity  REAL,
    status      TEXT DEFAULT 'auto',  -- 'auto' | 'confirmed' | 'rejected'
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### การเชื่อมกับ Grouped Events ที่มีอยู่

ระบบ grouping ที่มีอยู่แล้วสามารถใช้เป็น **training data** สำหรับ register faces ได้เลย:

```text
User groups events as "สมชาย"
  → GET /api/training-data
  → สำหรับแต่ละ group ที่ label=person:
      → crop faces จาก thumbnails
      → POST /api/face/register { name: group_name, images: [...] }
  → face-service เก็บ embeddings ของ "สมชาย" ไว้ใน gallery
  → events ถัดไปที่ detect person → auto-identify
```

---

## Part 2: Tapo FR Bridge (ดึงจากกล้อง Tapo)

### Tapo Camera ที่รองรับ Face Recognition

| รุ่น | Face Detection | Face Recognition | AI Chip |
|------|---------------|-----------------|---------|
| C225 | ✅ | ✅ (ตั้งชื่อได้ 50 คน) | ในตัว |
| C325WB | ✅ | ✅ | ในตัว |
| C720 | ✅ | ✅ | ในตัว |
| TC72 | ✅ | ❌ (detect อย่างเดียว) | ในตัว |
| TC70 | ❌ | ❌ | ไม่มี |

### วิธีดึงข้อมูล Face Recognition จาก Tapo

Tapo ใช้ **proprietary protocol** (ไม่ใช่ ONVIF มาตรฐาน) แต่มี library ที่ reverse engineer ได้แล้ว:

#### ช่องทางที่ 1: pytapo (Python library)

```python
from pytapo import Tapo

camera = Tapo("192.168.1.50", "admin", "camera_password")

# ดึงรายชื่อคนที่กล้องจำได้
faces = camera.getAIFaceList()
# Returns: [{"id": 1, "name": "สมชาย", "photo": <bytes>}, ...]

# ดึง detection events
events = camera.getDetectionEvents()
# Returns events ที่มี face_id ถ้าจำได้

# ดึง notification events (รวม face recognition)
notifications = camera.getNotifications(start_time, end_time)
```

#### ช่องทางที่ 2: ONVIF Event Subscription

Tapo รุ่นใหม่รองรับ ONVIF profile S บางส่วน:

```python
from onvif import ONVIFCamera

cam = ONVIFCamera("192.168.1.50", 2020, "admin", "password")
event_service = cam.create_events_service()

# Subscribe to motion/face events
# Tapo จะส่ง event เมื่อ detect/recognize face
pullpoint = event_service.CreatePullPointSubscription()
```

> **ข้อจำกัด:** ONVIF ของ Tapo ส่งแค่ event notification ไม่ส่ง face identity data ผ่าน ONVIF ต้องใช้ pytapo เพื่อดึง face identity

#### ช่องทางที่ 3: Tapo HTTPS API (Direct)

```
POST https://192.168.1.50/stok=<token>/ds
{
  "method": "get",
  "ai_detection": {
    "name": ["face_recognition"]
  }
}
```

### Tapo FR Bridge Service

```yaml
# เพิ่มใน docker-compose.yml
tapo-bridge:
  build:
    context: ./services/tapo-bridge
    dockerfile: Dockerfile
  environment:
    - TAPO_HOST=${TAPO_CAMERA_HOST}
    - TAPO_USER=${TAPO_CAMERA_USER}
    - TAPO_PASS=${TAPO_CAMERA_PASS}
    - POLL_INTERVAL=5               # วินาที
    - FACE_SERVICE_URL=http://face-service:8082
    - SPACEGUARDIAN_URL=http://squareguardian:8080
  depends_on:
    - face-service
  restart: unless-stopped
```

### Tapo Bridge Flow

```text
┌────────────────────────────────────────────────────────┐
│                   Tapo FR Bridge                        │
│                                                        │
│  1. Poll Tapo camera ทุก N วินาที                       │
│     └── pytapo.getDetectionEvents()                    │
│     └── pytapo.getAIFaceList()                         │
│                                                        │
│  2. เมื่อได้ face event:                                │
│     ├── ถ้า Tapo จำได้ (มี face_id + name):             │
│     │   └── Sync ไปยัง Unified Registry                │
│     │       POST /api/face/register-from-tapo           │
│     │       { tapo_face_id: 1, name: "สมชาย" }         │
│     │                                                  │
│     └── ถ้า Tapo จำไม่ได้ (unknown face):               │
│         └── ส่งภาพไป face-service ให้ลอง match          │
│             POST /api/face/identify { image: <crop> }   │
│                                                        │
│  3. อัปเดต SpaceGuardian event                          │
│     └── POST /api/annotate                             │
│         { event_id, identity, source: "tapo" }         │
└────────────────────────────────────────────────────────┘
```

---

## Part 3: Unified Identity Registry

### หลักการ

ไม่ว่า face จะถูกจดจำจากระบบไหน ข้อมูลจะรวมอยู่ใน registry เดียว:

```text
┌─────────────────────────────────────────────┐
│          Unified Identity Registry           │
│                                             │
│  Person: "สมชาย"                             │
│  ├── Embeddings จาก SpaceGuardian FR (5 รูป) │
│  ├── Embeddings จาก Tapo camera (3 รูป)      │
│  ├── Source: manual + tapo + auto            │
│  └── Total confidence: high                  │
│                                             │
│  Person: "คนส่งพัสดุ"                         │
│  ├── Embeddings จาก SpaceGuardian FR (2 รูป) │
│  ├── Source: manual                          │
│  └── Total confidence: medium                │
└─────────────────────────────────────────────┘
```

### Sync Strategy: Tapo ↔ SpaceGuardian

```text
กรณี A: ลงทะเบียนใน SpaceGuardian ก่อน
  1. User register "สมชาย" ผ่าน web UI
  2. face-service เก็บ embeddings
  3. Tapo Bridge sync ชื่อ + รูปไปยัง Tapo camera (ถ้ารองรับ)
     └── pytapo.setFaceName(face_id, "สมชาย")

กรณี B: Tapo จำได้ก่อน
  1. Tapo detect + recognize "Person A"
  2. Tapo Bridge poll แล้วเห็น face event
  3. ดึงรูป face จาก Tapo → register ใน face-service
  4. SpaceGuardian มี embedding ของ "Person A" แล้ว

กรณี C: ทั้งสองจำได้ → merge
  1. SpaceGuardian FR จำได้ว่าเป็น "สมชาย" (similarity 0.82)
  2. Tapo ก็จำได้ว่าเป็น face_id=1 "สมชาย"
  3. ทั้งสองยืนยันซึ่งกันและกัน → confidence สูงขึ้น
```

---

## Part 4: การ Implement เป็นขั้นตอน

### Phase 3A: Face Detection + Gallery (Next priority)

```
สิ่งที่ทำ:
  ✦ สร้าง face-service (Python + InsightFace)
  ✦ API: detect, register, identify
  ✦ SQLite embedding store
  ✦ เชื่อม Frigate events → auto face detect
  ✦ Web UI: หน้า Face Gallery (ดู/เพิ่ม/ลบคน)

ผลลัพธ์:
  → ระบบจำหน้าคนได้จาก RTSP stream ทุกกล้อง
  → ไม่ต้องพึ่ง Tapo AI

ระยะเวลาโดยประมาณ: 1-2 สัปดาห์
```

### Phase 3B: Tapo FR Bridge (เสริม)

```
สิ่งที่ทำ:
  ✦ สร้าง tapo-bridge service
  ✦ Poll face events จาก Tapo ผ่าน pytapo
  ✦ Sync Tapo faces ↔ SpaceGuardian gallery
  ✦ Fallback: ถ้า Tapo จำไม่ได้ → ส่งให้ SpaceGuardian FR ลอง

ผลลัพธ์:
  → ใช้ AI ของ Tapo เสริมการจดจำ
  → ลด load บน server (Tapo ทำ inference บนกล้อง)

ระยะเวลาโดยประมาณ: 1 สัปดาห์ (หลัง 3A เสร็จ)
```

### Phase 3C: Unified Identity + Auto-learning

```
สิ่งที่ทำ:
  ✦ Merge embeddings จากหลาย source
  ✦ Confidence scoring (multi-source confirmation)
  ✦ Auto-learning: user confirm → เพิ่ม embedding อัตโนมัติ
  ✦ เชื่อมกับ grouped events → batch register

ผลลัพธ์:
  → ระบบฉลาดขึ้นเรื่อยๆ จากการใช้งาน
  → ลด manual work ของผู้ใช้
```

---

## โครงสร้างไฟล์ใหม่

```text
squareguardian/
├── services/
│   ├── face/                          # SpaceGuardian FR service
│   │   ├── Dockerfile
│   │   ├── requirements.txt           # insightface, onnxruntime, fastapi, uvicorn
│   │   ├── main.py                    # FastAPI app
│   │   ├── face_engine.py             # InsightFace wrapper
│   │   ├── embedding_store.py         # SQLite operations
│   │   └── models/                    # downloaded ONNX models (gitignored)
│   │
│   └── tapo-bridge/                   # Tapo FR Bridge service
│       ├── Dockerfile
│       ├── requirements.txt           # pytapo, requests
│       ├── main.py                    # polling loop
│       ├── tapo_client.py             # pytapo wrapper
│       └── sync.py                    # gallery sync logic
│
├── storage/
│   └── face-db/                       # SQLite + face data
│       └── .gitkeep
│
├── config/
│   └── face/
│       └── config.yml                 # model settings, thresholds
```

---

## ข้อเปรียบเทียบ

| ด้าน | SpaceGuardian FR | Tapo FR |
|------|-----------------|---------|
| **กล้องที่รองรับ** | ทุกกล้อง RTSP | เฉพาะ Tapo รุ่นที่มี AI |
| **จำนวนคนที่จำได้** | ไม่จำกัด (ตาม storage) | 50 คน (ข้อจำกัดของ Tapo) |
| **ความแม่นยำ** | สูง (ArcFace pretrained) | ดี (แต่ไม่เปิดเผย model) |
| **Resource ที่ใช้** | CPU/GPU บน server | ไม่ใช้ server (ทำบนกล้อง) |
| **Latency** | ~100ms (CPU) / ~20ms (GPU) | realtime (บนกล้อง) |
| **ความยืดหยุ่น** | เต็มที่ (เลือก model, threshold) | จำกัด (ตาม firmware) |
| **Offline** | ✅ ทำงาน local ทั้งหมด | ✅ ทำงาน local ทั้งหมด |

### เมื่อใช้ร่วมกัน

- **Tapo ทำ first-pass** → detect + recognize บนกล้อง (ไม่เสีย server resource)
- **SpaceGuardian FR ทำ second-pass** → verify หรือ identify คนที่ Tapo จำไม่ได้
- **Cross-validation** → ทั้งสองยืนยันซึ่งกันและกัน → confidence สูงขึ้น
- **Fallback** → ถ้ากล้องไม่ใช่ Tapo หรือ Tapo ไม่มี AI → SpaceGuardian FR ทำเองทั้งหมด

---

## ข้อควรพิจารณา

### ความเป็นส่วนตัว (Privacy)

- ข้อมูลใบหน้าทั้งหมดเก็บ **local เท่านั้น** ไม่ส่งออก cloud
- Embeddings ไม่สามารถ reverse กลับเป็นภาพได้
- ผู้ใช้สามารถลบข้อมูลใบหน้าได้ทุกเมื่อ
- ควรมี consent notice สำหรับพื้นที่สาธารณะ

### Performance บน CPU

- InsightFace buffalo_s: ~100ms ต่อ face บน CPU ทั่วไป
- เพียงพอสำหรับ 1-5 กล้อง ถ้ามากกว่านี้แนะนำ GPU
- Tapo FR ไม่ใช้ resource ของ server เลย → ช่วยลด load

### ข้อจำกัดของ Tapo Integration

- pytapo เป็น reverse-engineered library → อาจเปลี่ยนเมื่อ firmware update
- ไม่ได้ใช้ official API (Tapo ไม่เปิด public API)
- บางรุ่นอาจไม่รองรับ face recognition API
- ควรออกแบบให้ Tapo Bridge เป็น **optional** ไม่ใช่ dependency

---

## สรุป

| สิ่งที่ได้ | รายละเอียด |
|-----------|-----------|
| **ระบบของเรา** | Face recognition ที่ใช้กับทุกกล้อง ควบคุมได้เต็มที่ |
| **ดึง Tapo AI** | ใช้ AI ที่ Tapo มีในตัว ลด server load |
| **Unified Registry** | ข้อมูลจากทุก source รวมที่เดียว |
| **Progressive** | เริ่มจาก face-service ก่อน → เพิ่ม Tapo bridge ทีหลัง |
| **Privacy-first** | ข้อมูลอยู่ local ทั้งหมด |
| **Extensible** | เพิ่มกล้องยี่ห้ออื่นได้ในอนาคต (Hikvision, Dahua bridge) |
