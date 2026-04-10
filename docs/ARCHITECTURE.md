# ARCHITECTURE.md

## ภาพรวม

`SquareGuardian` เป็นระบบ AI Video Intelligence แบบ self-hosted ที่ตอบคำถาม **Who, What, Where, When** จากกล้อง RTSP/IP ได้แบบ real-time

## System Architecture

```text
┌─────────────┐     RTSP      ┌──────────────┐   MQTT Events   ┌─────────────────────┐
│ IP Cameras  │──────────────>│   Frigate    │───────────────>│  SquareGuardian     │
│ (1-4 cams)  │               │   (NVR)      │                │  Event Engine (Go)  │
└─────────────┘               └──────┬───────┘                └──────┬──────────────┘
                                     │                                │
                              ┌──────┴───────┐               ┌───────┴────────┐
                              │  go2rtc      │               │                │
                              │  (restream)  │               │  SQLite DB     │
                              └──────┬───────┘               │  (events,      │
                                     │                        │   persons,     │
                              RTSP restream                   │   vehicles)    │
                                     │                        └───────┬────────┘
                              ┌──────┴───────┐                        │
                              │  Action      │                ┌───────┴────────┐
                              │  Recognition │                │  REST API      │
                              │  (Python)    │                │  + WebSocket   │
                              └──────────────┘                └───────┬────────┘
                                                                      │
┌─────────────┐                                               ┌───────┴────────┐
│ CompreFace  │<──── face crop ─────── SquareGuardian ──────>│  Notifications │
│ (Face ID)   │                        Engine                 │  LINE/Telegram │
└─────────────┘                                               │  /Webhook      │
                                                              └────────────────┘
                              ┌──────────────┐
                              │  Mosquitto   │
                              │  MQTT Broker │
                              └──────────────┘
```

## Service Breakdown

| Service | Image/Build | Port | Role |
|---------|------------|------|------|
| **mosquitto** | `eclipse-mosquitto:2` | 1883 | MQTT broker สำหรับ event communication |
| **frigate** | `ghcr.io/blakeblackshear/frigate:stable` | 8971, 5001, 8554 | NVR + object detection (person, vehicle) |
| **compreface-core** | `exadel/compreface-core:1.2.0` | - | Face recognition ML engine |
| **compreface-api** | `exadel/compreface-api:1.2.0` | - | CompreFace REST API |
| **compreface-admin** | `exadel/compreface-admin:1.2.0` | 8000 | CompreFace admin UI |
| **compreface-postgres** | `postgres:16-alpine` | - | CompreFace database |
| **squareguardian** | Build from `./` | 8080 | Central Event Engine (Go) |
| **action-recognition** | Build from `./services/action-recognition` | 8083 | Skeleton-based action classification |
| **face-service** | Build from `./services/face` | 8082 | Legacy face service (optional) |

## Data Flow Pipeline

```text
1. กล้อง RTSP → Frigate (detect person/car/motorcycle)
2. Frigate → MQTT event → SquareGuardian Engine
3. Engine receives event:
   a. If person detected + has snapshot:
      → Fetch snapshot from Frigate
      → Send to CompreFace → Get face match (Who)
   b. Action Recognition service (parallel):
      → Reads RTSP restream from go2rtc
      → Pose estimation (MediaPipe) → Skeleton → Classify action (What)
      → Publish action event to MQTT
   c. Engine correlates:
      → Who (CompreFace match) + What (action) + Where (camera/zone) + When (timestamp)
      → Store unified event in SQLite
      → Broadcast via WebSocket to dashboard
      → Evaluate alert rules → Send notifications
```

## Event Schema (unified)

```json
{
  "id": "1234567890.abcdef",
  "frigate_id": "1234567890.abcdef",
  "camera": "cam_front",
  "label": "person",
  "top_score": 0.92,
  "start_time": "2026-04-10T22:14:00Z",
  "end_time": "2026-04-10T22:14:30Z",
  "zone": "front_door",
  "person_id": "p001",
  "person_name": "Somchai",
  "face_score": 0.95,
  "action": "walk",
  "action_score": 0.87,
  "vehicle_id": "",
  "plate": "",
  "snapshot_path": "/api/events/1234567890.abcdef/snapshot.jpg",
  "alert_sent": false,
  "alert_type": "",
  "created_at": "2026-04-10T22:14:00.123Z"
}
```

## API Endpoints

### V2 API (Event Engine — MQTT + SQLite)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v2/events?camera=&label=&limit=&offset=` | Query unified events |
| GET | `/api/v2/events/{id}` | Get single event |
| GET | `/api/v2/persons` | List registered persons |
| POST | `/api/v2/persons` | Register a person |
| GET | `/api/v2/vehicles` | List registered vehicles |
| POST | `/api/v2/vehicles` | Register a vehicle |
| GET | `/api/v2/stats` | Event count + WebSocket connections |
| WS | `/ws` | Real-time event stream (WebSocket) |

### V1 API (Legacy polling-based — backward compatible)

เดิมที่มีอยู่ยังใช้งานได้ปกติ

## Module Responsibilities

| Module | Answers | Implementation |
|--------|---------|----------------|
| **Detection & Tracking** | What is visible? | Frigate (YOLO/SSD on CPU/TPU) |
| **Face Identification** | Who is it? | CompreFace REST API |
| **Action Recognition** | What are they doing? | MediaPipe Pose → skeleton classifier |
| **Vehicle / LPR** | What vehicle/plate? | Frigate labels + future plate OCR |
| **Rules / Alerts** | Should we notify? | Engine alert rules + cooldown |
| **Storage / Timeline** | When did it happen? | SQLite + event log |
| **Notifications** | Notify who? | LINE Notify / Telegram / Webhook |

## หลักการออกแบบ

- **MVP ก่อน** — ทำให้ใช้ได้จริงก่อนค่อยขยาย
- **Config-driven** — ใช้ .env และ YAML, ไม่ hardcode
- **Modular** — แต่ละ service แยกกัน, สื่อสารผ่าน MQTT/HTTP
- **Local-first** — ทำงานได้โดยไม่ต้อง cloud
- **Upgrade-friendly** — เปลี่ยน detector, เพิ่ม GPU/NPU ได้ภายหลัง

## Supported Hardware

- **Intel N100/N305** + iGPU + Coral USB TPU
- **NVIDIA Jetson** (Nano, Xavier NX, Orin Nano)
- **Any x86/ARM64** ที่รัน Docker ได้
- Windows (Docker Desktop + WSL2) สำหรับ dev
