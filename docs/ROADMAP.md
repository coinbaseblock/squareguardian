# ROADMAP.md

## เป้าหมาย

Roadmap สำหรับ `SquareGuardian` — local AI video intelligence platform ที่ตอบ **Who, What, Where, When** จากกล้อง RTSP/IP

## Phase 0 — Repo + Tapo Starter (DONE)

- [x] Docker Compose สำหรับ local pilot
- [x] Frigate config สำหรับกล้อง RTSP 1 ตัว
- [x] `.env.example` สำหรับเก็บค่า RTSP นอก repo
- [x] README ที่เริ่มใช้งานได้จริง

## Phase 1 — Infrastructure + Core Services (CURRENT)

- [x] Mosquitto MQTT broker container
- [x] Frigate MQTT events enabled
- [x] CompreFace containers (core + api + admin + postgres)
- [x] SQLite schema design (events, persons, vehicles, cameras, alert_rules)
- [x] Go service scaffold with MQTT subscriber
- [x] Event Engine: correlate Who + What + Where + When
- [x] REST API v2: query events, persons, vehicles
- [x] WebSocket: real-time event push
- [ ] ทดสอบ full stack กับกล้องจริง

## Phase 2 — Detection + Face Identification

- [x] Frigate object detection (person/vehicle)
- [x] CompreFace client (Go) — recognize + add subject
- [ ] ลงทะเบียนหน้าบุคคลที่รู้จักใน CompreFace admin UI
- [ ] เชื่อม Frigate face crop → CompreFace API (auto on person event)
- [ ] Unknown face alert logic (similarity < threshold)
- [ ] ทดสอบ face recognition accuracy

## Phase 3 — Action Recognition

- [x] Action recognition service scaffold (Python + MediaPipe)
- [x] MQTT publish action events
- [x] REST API สำหรับ classify frame
- [ ] ทดสอบกับ RTSP stream จริง
- [ ] Fall detection tuning
- [ ] Loitering timer (person stationary > threshold)
- [ ] เทรน custom DNN classifier แทน rule-based

## Phase 4 — Notifications + Alerts

- [x] LINE Notify integration
- [x] Telegram Bot integration
- [x] Webhook integration
- [x] Alert cooldown / dedup logic
- [ ] Alert rules config (per camera, per zone, per type)
- [ ] Alert rules management API

## Phase 5 — Dashboard

- [ ] Live camera grid (HLS/RTSP via go2rtc)
- [ ] Event timeline view
- [ ] Person gallery (known / unknown)
- [ ] Vehicle log with plate snapshots
- [ ] Real-time event feed (WebSocket)

## Phase 6 — Vehicle / LPR

- [ ] Frigate LPR config / plate recognizer integration
- [ ] Vehicle registry + plate lookup
- [ ] Known / unknown vehicle alerts
- [ ] Watchlist / whitelist / blacklist

## Phase 7 — Access-control Extensions

- [ ] Guest-mapping (hotel use case)
- [ ] Schedule-based access rules
- [ ] Factory/office presets
- [ ] Attendance inference from identity + line crossing

## Phase 8 — Hardening

- [ ] Auth for dashboard + API
- [ ] Retention policy (auto-purge old events/clips)
- [ ] Health checks + auto-restart
- [ ] Documentation / README update
- [ ] GPU/NPU acceleration (OpenVINO, TensorRT)

## Implementation Checklist

| # | หมวด | รายการ | สถานะ |
|---|------|--------|-------|
| 1.1 | Infra | Docker Compose skeleton (network, volumes) | ✅ |
| 1.2 | Infra | Mosquitto MQTT broker container | ✅ |
| 1.3 | Infra | SQLite schema design | ✅ |
| 2.1 | NVR | Frigate container + MQTT events | ✅ |
| 2.2 | NVR | ทดสอบ object detection กับกล้องจริง | ⬜ |
| 2.3 | NVR | MQTT events จาก Frigate | ✅ |
| 2.4 | LPR | ตั้งค่า Frigate LPR | ⬜ |
| 3.1 | Face | CompreFace container + API | ✅ |
| 3.2 | Face | ลงทะเบียนหน้าบุคคล | ⬜ |
| 3.3 | Face | Frigate face crop → CompreFace API | ✅ |
| 3.4 | Face | Unknown face alert logic | ✅ |
| 4.1 | Action | Action recognition container | ✅ |
| 4.2 | Action | รับ RTSP stream จาก go2rtc restream | ✅ |
| 4.3 | Action | ส่ง action events เข้า MQTT | ✅ |
| 4.4 | Action | Fall detection + loitering timer | ⬜ |
| 5.1 | Engine | Go service scaffold (MQTT + SQLite) | ✅ |
| 5.2 | Engine | MQTT subscriber: Frigate events | ✅ |
| 5.3 | Engine | Correlation logic: Who + What + Where + When | ✅ |
| 5.4 | Engine | SQLite event storage | ✅ |
| 5.5 | Engine | REST API: query events, persons, vehicles | ✅ |
| 5.6 | Engine | WebSocket: real-time event push | ✅ |
| 6.1 | UI | Live camera grid | ⬜ |
| 6.2 | UI | Event timeline view | ⬜ |
| 6.3 | UI | Person gallery | ⬜ |
| 6.4 | UI | Vehicle log | ⬜ |
| 7.1 | Alert | LINE Notify / Telegram integration | ✅ |
| 7.2 | Alert | Alert rules config | ⬜ |
| 7.3 | Alert | Cooldown / dedup logic | ✅ |
| 8.1 | Ops | Auth for dashboard + API | ⬜ |
| 8.2 | Ops | Retention policy | ⬜ |
| 8.3 | Ops | Health checks + auto-restart | ⬜ |
| 8.4 | Ops | Documentation / README | ⬜ |

## Risks & Limitations

1. **CompreFace accuracy** — ขึ้นกับคุณภาพ snapshot จาก Frigate, ระยะห่างจากกล้อง
2. **Action recognition** — rule-based classifier เป็นแค่ placeholder, ต้องเทรน DNN สำหรับ production
3. **CPU load** — action recognition + pose estimation หนัก, ควรจำกัด FPS
4. **Single-writer SQLite** — เพียงพอสำหรับ 1-4 กล้อง, ถ้าขยายต้องย้ายไป PostgreSQL
5. **No auth yet** — API/WebSocket ยังไม่มี authentication
