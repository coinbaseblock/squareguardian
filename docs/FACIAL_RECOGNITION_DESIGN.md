# Local Facial Recognition Design

## ภาพรวม

SpaceGuardian จะมีระบบ Facial Recognition แบบ **Local-first** โดยมีแนวทางหลัก:

1. **SpaceGuardian FR** — ระบบจดจำใบหน้าของเราเอง (InsightFace/ArcFace) รันบน server
2. **Tapo ONVIF Trigger** — ใช้ ONVIF person detection จาก Tapo เป็น trigger เพื่อลด processing
3. **Tapo FR Bridge (อนาคต)** — ดึงผล face recognition จาก Tapo (รอ API เปิด)

> **ข้อค้นพบสำคัญ:** Tapo ที่มี facial recognition (C260, C560WS, H500 hub) **ไม่เปิด API**
> ให้ดึงผลจดจำใบหน้าได้ — ทั้ง ONVIF, pytapo, และ REST API ยังไม่รองรับ
> ดังนั้นแนวทางหลักคือ **สร้างระบบ FR ของเราเอง** ที่ใช้ได้กับทุกกล้อง RTSP

---

## สถาปัตยกรรม

```text
                    ┌─────────────────────────────────────┐
                    │        Unified Identity Registry     │
                    │   (embedding store + person profile) │
                    └──────────────────┬──────────────────┘
                                       │
                          ┌────────────▼────────────┐
                          │    SpaceGuardian FR      │
                          │    (Self-hosted)         │
                          │                          │
                          │  InsightFace / ArcFace   │
                          │  ONNX Runtime            │
                          └────────────┬────────────┘
                                       │
                    ┌──────────────────▼──────────────────┐
                    │          Trigger Sources             │
                    │                                     │
                    │  ┌─────────────┐  ┌──────────────┐  │
                    │  │ Frigate     │  │ ONVIF Person │  │
                    │  │ person event│  │ Detection    │  │
                    │  │ (primary)   │  │ (secondary)  │  │
                    │  └──────┬──────┘  └──────┬───────┘  │
                    └─────────┼────────────────┼──────────┘
                              │                │
                    ┌─────────▼────────────────▼──────────┐
                    │     RTSP Stream จากกล้องทุกยี่ห้อ     │
                    │     (Tapo, Hikvision, Dahua, ฯลฯ)   │
                    └────────────────────────────────────┘
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
    source      TEXT DEFAULT 'manual', -- 'manual' | 'group' | 'auto'
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE face_embeddings (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    person_id   TEXT REFERENCES persons(id),
    embedding   BLOB NOT NULL,       -- 512-dim float32 = 2048 bytes
    source      TEXT DEFAULT 'manual', -- 'manual' | 'frigate' | 'group'
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

## Part 2: Tapo Integration — สิ่งที่ทำได้จริงตอนนี้

### สถานะ Facial Recognition ของ Tapo (ข้อเท็จจริง)

| รุ่น | Person Detection | Face Recognition | API เปิดให้ดึง FR? |
|------|-----------------|-----------------|-------------------|
| C260 | ✅ | ✅ | ❌ ไม่เปิด |
| C560WS | ✅ | ✅ | ❌ ไม่เปิด |
| H500 hub | ✅ (6 กล้อง) | ✅ | ❌ ไม่เปิด |
| C225 | ✅ | ❌ | — |
| C325WB | ✅ | ❌ | — |
| TC72 | ✅ | ❌ | — |
| TC70 | ✅ (basic) | ❌ | — |

> **ข้อจำกัดสำคัญ:** Tapo **ไม่เปิด API** สำหรับดึงผล facial recognition
> - pytapo ไม่มี method สำหรับ face recognition data
> - ONVIF ของ Tapo ส่งแค่ person/motion detection events ไม่มี face identity
> - REST API ของ Tapo ยังไม่ถูก reverse-engineer ส่วน face recognition
> - HomeAssistant-Tapo-Control มี [feature request #1043](https://github.com/JurajNyiri/HomeAssistant-Tapo-Control/issues/1043) แต่ยังไม่ implement

### สิ่งที่ดึงจาก Tapo ได้จริงตอนนี้

#### 1. ONVIF Person Detection → ใช้เป็น Trigger

Tapo ส่ง ONVIF events ได้ผ่าน port 2020:

```text
ONVIF Event Topics ที่ Tapo รองรับ:
  tns1:RuleEngine/CellMotionDetector/Motion    → IsMotion (boolean)
  tns1:RuleEngine/PeopleDetector/People        → IsPeople (boolean)
  tns1:RuleEngine/LineCrossDetector/LineCross  → IsLineCross (boolean)
  tns1:RuleEngine/TamperDetector/Tamper        → IsTamper (boolean)
```

**ใช้ IsPeople เป็น trigger** → เมื่อ Tapo detect person → grab RTSP frame → ส่งให้ SpaceGuardian FR ทำ face recognition:

```text
Tapo Camera (ONVIF port 2020)
  │
  ├── IsPeople = true  ──────────┐
  │                               ▼
  │                    SpaceGuardian FR
  │                    face-service
  ├── RTSP stream ──────────────▶ crop face + identify
  │                               │
  │                               ▼
  │                    "สมชาย" (similarity: 0.87)
```

> **หมายเหตุ:** ต้องเปิด Motion Detection ใน Tapo App ด้วย มิฉะนั้น ONVIF person event จะไม่ถูกส่ง (known firmware bug)

#### 2. pytapo — ตั้งค่า detection config

```python
from pytapo import Tapo

camera = Tapo("192.168.1.50", "admin", "camera_password")

# ดึงและตั้งค่า person detection (config เท่านั้น ไม่ใช่ผลลัพธ์)
config = camera.getPersonDetectionConfig()
camera.setMotionDetection(True, "high")
info = camera.getBasicInfo()
```

pytapo ทำได้: ตั้งค่า detection, ดูข้อมูลกล้อง, pan/tilt, privacy mode
pytapo ทำ **ไม่ได้**: ดึงผล face recognition, ดึง face events

#### 3. RTSP Stream → ตัวหลักสำหรับ FR

ช่องทางที่ reliable ที่สุดคือดึง RTSP stream แล้วรัน face recognition ของเราเอง:

```text
rtsp://username:password@192.168.1.50/stream1  →  Frigate  →  face-service
```

### Tapo ONVIF Trigger Service (ทำได้เลย)

แทนที่จะเป็น "Tapo FR Bridge" ที่ดึง face data จาก Tapo (ซึ่งทำไม่ได้)
เราจะทำ **ONVIF Trigger Service** ที่ใช้ Tapo person detection เป็น trigger แทน:

```yaml
# เพิ่มใน docker-compose.yml (optional — ลด CPU load)
onvif-trigger:
  build:
    context: ./services/onvif-trigger
    dockerfile: Dockerfile
  environment:
    - CAMERA_HOST=${TAPO_CAMERA_HOST}
    - ONVIF_PORT=2020
    - ONVIF_USER=${TAPO_CAMERA_USER}
    - ONVIF_PASS=${TAPO_CAMERA_PASS}
    - FACE_SERVICE_URL=http://face-service:8082
    - SPACEGUARDIAN_URL=http://squareguardian:8080
  depends_on:
    - face-service
  restart: unless-stopped
```

**ข้อดี:** ลด CPU load — ไม่ต้องรัน face detection ตลอดเวลา แค่รันเมื่อ Tapo detect person

### อนาคต: Tapo FR Bridge (เมื่อ API เปิด)

เก็บ design ไว้สำหรับอนาคต — ถ้า pytapo หรือ community reverse-engineer ได้:

```text
สิ่งที่ต้องรอ:
  ⏳ pytapo เพิ่ม getAIFaceList() / getFaceRecognitionEvents()
  ⏳ HomeAssistant-Tapo-Control issue #1043 ถูก implement
  ⏳ TP-Link เปิด official API (ไม่น่าจะเกิดเร็ว)

เมื่อพร้อม → สร้าง tapo-bridge service ที่:
  1. ดึง face recognition results จาก Tapo
  2. Sync ชื่อคนที่จำได้เข้า SpaceGuardian gallery
  3. Cross-validate กับ SpaceGuardian FR
```

---

## Part 3: Unified Identity Registry

### หลักการ

ข้อมูลใบหน้าทั้งหมดรวมอยู่ใน registry เดียว ไม่ว่าจะมาจาก source ไหน:

```text
┌──────────────────────────────────────────────────┐
│            Unified Identity Registry              │
│                                                  │
│  Person: "สมชาย"                                  │
│  ├── Embeddings จาก manual register (3 รูป)       │
│  ├── Embeddings จาก Frigate auto-capture (8 รูป)  │
│  ├── Embeddings จาก grouped events (5 รูป)        │
│  ├── Source: manual + frigate + group             │
│  └── Total confidence: high (16 samples)          │
│                                                  │
│  Person: "คนส่งพัสดุ"                              │
│  ├── Embeddings จาก manual register (2 รูป)       │
│  ├── Source: manual                              │
│  └── Total confidence: medium (2 samples)         │
│                                                  │
│  [อนาคต] Person: "แม่บ้าน"                        │
│  ├── Embeddings จาก Tapo FR Bridge (เมื่อ API เปิด)│
│  └── Source: tapo                                │
└──────────────────────────────────────────────────┘
```

### Identity Enrichment Flow

```text
กรณี A: ลงทะเบียนด้วยมือ
  1. User upload รูปหน้า "สมชาย" ผ่าน web UI
  2. face-service extract embeddings → เก็บใน gallery
  3. เมื่อ Frigate detect person → face-service จำได้ → auto-annotate

กรณี B: ใช้ Grouped Events ที่มีอยู่
  1. User group events ว่า "นี่คือสมชาย" (ระบบที่มีอยู่แล้ว)
  2. กด "Register Face" → crop faces จาก thumbnails
  3. face-service เก็บ embeddings ของ "สมชาย"
  4. events ถัดไป → auto-identify

กรณี C: Auto-learning
  1. face-service จำได้ → suggest "อาจเป็นสมชาย"
  2. User กด confirm → เพิ่ม embedding ใหม่เข้า gallery
  3. ระบบแม่นยำขึ้นเรื่อยๆ (more samples = better)

กรณี D: อนาคต — Tapo FR sync
  1. เมื่อ Tapo เปิด API → Tapo Bridge ดึง face identity
  2. Sync เข้า Unified Registry
  3. Cross-validate กับ embeddings ที่มีอยู่
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

### Phase 3B: ONVIF Trigger + Smart Processing (เสริม)

```
สิ่งที่ทำ:
  ✦ สร้าง onvif-trigger service (optional)
  ✦ Subscribe ONVIF IsPeople event จาก Tapo
  ✦ เมื่อ Tapo detect person → trigger face-service ทำ FR
  ✦ ลด CPU load (ไม่ต้อง process ทุก frame)

ผลลัพธ์:
  → ใช้ person detection ของ Tapo เป็น trigger (ฟรี ไม่เสีย server resource)
  → face-service รันเฉพาะเมื่อมีคนจริงๆ
  → รองรับกล้อง ONVIF ยี่ห้ออื่นด้วย

หมายเหตุ:
  → Frigate ทำ person detection ได้อยู่แล้ว service นี้จึง optional
  → มีประโยชน์เมื่อต้องการลด CPU load บน server
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
│   └── onvif-trigger/                 # ONVIF person detection trigger (optional)
│       ├── Dockerfile
│       ├── requirements.txt           # onvif-zeep, requests
│       ├── main.py                    # ONVIF event subscription loop
│       └── trigger.py                 # trigger face-service on person event
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

| ด้าน | SpaceGuardian FR (ของเรา) | Tapo FR (ในตัวกล้อง) |
|------|--------------------------|---------------------|
| **กล้องที่รองรับ** | ทุกกล้อง RTSP | เฉพาะ C260, C560WS, + H500 hub |
| **จำนวนคนที่จำได้** | ไม่จำกัด (ตาม storage) | ~50 คน |
| **ดึงผลผ่าน API ได้** | ✅ เต็มที่ | ❌ ไม่เปิด API |
| **ความแม่นยำ** | สูง (ArcFace pretrained) | ดี (แต่ไม่เปิดเผย model) |
| **Resource ที่ใช้** | CPU/GPU บน server | ไม่ใช้ server (ทำบนกล้อง) |
| **Latency** | ~100ms (CPU) / ~20ms (GPU) | realtime (บนกล้อง) |
| **ความยืดหยุ่น** | เต็มที่ (เลือก model, threshold) | จำกัด (ดูผลผ่าน Tapo App เท่านั้น) |
| **Offline** | ✅ ทำงาน local ทั้งหมด | ✅ ทำงาน local ทั้งหมด |

### กลยุทธ์ที่ใช้ได้จริงตอนนี้

- **SpaceGuardian FR เป็นหลัก** → ทำ face recognition ทั้งหมดบน server ของเรา
- **Tapo ช่วยเป็น trigger** → ONVIF IsPeople event ลด unnecessary processing
- **Frigate เป็น backbone** → person detection + snapshot + event management
- **ไม่ต้องพึ่ง Tapo FR** → ระบบทำงานได้ครบโดยไม่ต้องอาศัย Tapo AI เลย
- **เตรียม interface ไว้** → เมื่อ Tapo เปิด API ในอนาคต สามารถ plug-in ได้ทันที

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

- Tapo **ไม่เปิด API** สำหรับ facial recognition results (walled garden)
- pytapo ทำได้แค่ตั้งค่า detection config ไม่สามารถดึง face identity
- ONVIF ส่งแค่ boolean person detection ไม่มี face identity data
- ONVIF person events มี bug: ต้องเปิด Motion Detection ใน Tapo App ด้วย
- ดังนั้น Tapo ใช้เป็น **trigger** ได้ แต่ไม่ใช่ **source** ของ face recognition
- ติดตาม [HomeAssistant-Tapo-Control #1043](https://github.com/JurajNyiri/HomeAssistant-Tapo-Control/issues/1043) สำหรับ updates

---

## สรุป

| สิ่งที่ได้ | รายละเอียด |
|-----------|-----------|
| **ระบบของเรา** | Face recognition ที่ใช้กับทุกกล้อง ควบคุมได้เต็มที่ |
| **ใช้ Tapo เป็น trigger** | ONVIF person detection ลด unnecessary processing |
| **Unified Registry** | ข้อมูลจากทุก source รวมที่เดียว |
| **Progressive** | เริ่มจาก face-service ก่อน → เพิ่ม Tapo bridge ทีหลัง |
| **Privacy-first** | ข้อมูลอยู่ local ทั้งหมด |
| **Extensible** | เพิ่มกล้องยี่ห้ออื่นได้ + plug-in Tapo FR เมื่อ API เปิดในอนาคต |
