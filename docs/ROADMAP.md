# ROADMAP.md

## เป้าหมาย

roadmap นี้จัดลำดับให้ `SquareGuardian` โตจาก single-camera local pilot ไปเป็นระบบ video intelligence ที่มี detection, identity, attendance, vehicle, lpr และ access-control โดยไม่เริ่มยากเกินไป.

## Phase 0 — Repo + Tapo Starter

สิ่งที่ควรมี:

- เปลี่ยน branding เป็น `SquareGuardian`
- Docker Compose สำหรับ local pilot
- Frigate config สำหรับกล้อง RTSP 1 ตัว
- `.env.example` สำหรับเก็บค่า RTSP นอก repo
- README ที่เริ่มใช้งานได้จริง

ผลลัพธ์:

- คนเปิด repo แล้ว start ได้เลย

## Phase 1 — Detection Pilot

สิ่งที่ทำ:

- detect `person`
- detect `vehicle`
- snapshots
- zone เบื้องต้น
- review ผ่าน UI
- live stream ใน browser ต้องมี fallback/transcode path สำหรับกล้องที่ปล่อย H.265/HEVC

ผลลัพธ์:

- ใช้งานเฝ้าระวังพื้นฐานได้จริง

## Phase 2 — Rule-based Alerts

สิ่งที่ทำ:

- `front-door`, `gate`, `parking` zones
- notifier แบบง่าย
- event logging
- after-hours / restricted area rules

ผลลัพธ์:

- เริ่มใช้งานแบบ alert-driven ได้

## Phase 3 — Registry Foundation

สิ่งที่ทำ:

- person registry
- vehicle registry
- normalized event schema
- known / unknown hooks

## Phase 4 — Identity / Attendance

สิ่งที่ทำ:

- face gallery
- known / unknown person
- entry/exit based attendance

## Phase 5 — Vehicle / LPR

สิ่งที่ทำ:

- plate OCR
- known / unknown vehicle
- watchlist / whitelist / blacklist

## Phase 6 — Access-control Extensions

สิ่งที่ทำ:

- guest-mapping
- schedule-based access rules
- hotel / factory presets

## ลำดับที่ควรทำจริง

1. เปิด stack ให้ได้
2. ให้ Tapo ขึ้นภาพและ detect คน/รถ
3. ค่อยเพิ่ม zone/alert
4. ค่อยเก็บ event log
5. ค่อยต่อ registry
6. ค่อยเพิ่ม identity และ lpr
