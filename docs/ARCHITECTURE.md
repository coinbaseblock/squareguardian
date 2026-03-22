# ARCHITECTURE.md

## ภาพรวม

`SpaceGuardian` เริ่มจากสถาปัตยกรรมเล็กที่สุดที่ยังใช้งานจริงได้กับ local pilot 1 กล้องก่อน แล้วค่อยแยก layer เพิ่มเมื่อมีความจำเป็น.

## สถาปัตยกรรม MVP ที่ใช้ตอนนี้

```text
TP-Link Tapo RTSP Camera
  -> Frigate
  -> Detection Event
  -> Zone Filter
  -> Snapshot / Review
  -> UI
```

## Layer ที่ควรมีในรอบแรก

### 1. Detection Layer

หน้าที่:

- รับ RTSP
- detect `person` และ `vehicle`
- ส่ง event พื้นฐานไปยัง review/UI

เครื่องมือเริ่มต้น:

- Frigate
- go2rtc restream สำหรับแยก stream ตรวจจับออกจาก stream live view ที่ browser เล่นได้ง่ายกว่า

สถานะ:

- **ทำจริงทันที**

### 2. Rule / Zone Layer

หน้าที่:

- กรองเฉพาะพื้นที่ที่สนใจ
- แยก `front-door`, `gate`, `parking`
- ลด noise จากพื้นที่ที่ไม่ต้องการ

สถานะ:

- **ทำจริงทันที**

### 3. Storage Layer

หน้าที่:

- เก็บ snapshot
- เก็บ media/event ที่จำเป็น
- เปิดทางให้ต่อ event logger ทีหลัง

สถานะ:

- **ทำจริงแบบเบา ๆ**

## Layer ที่ควรตามมาทีหลัง

### Registry Layer

- เก็บ person/vehicle metadata
- ใช้เป็นฐานของ known / unknown

### Identity Layer

- รู้ว่าเป็นใคร
- เชื่อม attendance ได้

### Vehicle / LPR Layer

- อ่านป้ายทะเบียน
- ผูกกับ vehicle registry

### Access-control / Guest-mapping Layer

- ใช้สำหรับ office, hotel, factory use case

## หลักการออกแบบ

- MVP ต้องใช้งานง่ายก่อน
- config ต้องแก้ได้ด้วยไฟล์ ไม่ hardcode
- หลีกเลี่ยงการบังคับ accelerator ตั้งแต่วันแรก
- แยก detection ออกจาก identity และ business rule
- ไม่เก็บ secret ใน repo

## Data flow ที่แนะนำเมื่อระบบโตขึ้น

```text
RTSP Camera
  -> Frigate
  -> Event Normalizer
  -> Registry Lookup
  -> Alert Rule
  -> Notifier
  -> Event Store
```

แต่ flow นี้ยังเป็น **phase ถัดไป** ไม่ใช่สิ่งที่ต้องทำให้ครบในรอบแรก
