# FEATURE_MATRIX.md

## จุดประสงค์

เอกสารนี้สรุปว่าอะไรควรทำในช่วงไหน โดยยึดหลักว่า **ทำของที่ใช้ได้จริงก่อน**

## สถานะที่ใช้ในเอกสารนี้

- **Now** = ควรทำในรอบแรก
- **Next** = ควรทำหลัง MVP เริ่มนิ่ง
- **Later** = เก็บไว้ต่อยอดภายหลัง

## ความสามารถหลัก

### Detection

- person detection — **Now**
- vehicle detection — **Now**
- face detection — **Next**
- bag detection — **Next**
- motorcycle / truck classification — **Next**
- helmet / PPE detection — **Later**
- fire / smoke detection — **Later**

### Identity

- person registry structure — **Now**
- known / unknown hook — **Next**
- face gallery matching — **Next**
- person re-identification — **Later**
- multi-angle robust identity — **Later**

### Behavior

- line crossing rules — **Now**
- restricted zone intrusion — **Now**
- after-hours movement — **Now**
- loitering — **Next**
- fall detection — **Next**
- fence climbing — **Later**
- tailgating — **Later**
- suspicious carrying — **Later**
- group anomaly — **Later**

### Attendance / Presence

- line-based entry / exit logs — **Now**
- presence duration from events — **Next**
- known-person attendance — **Next**
- late / early leave / absent — **Next**
- cross-camera attendance fusion — **Later**

### Vehicle / LPR

- vehicle event log — **Now**
- vehicle registry structure — **Now**
- known / unknown vehicle hook — **Next**
- LPR / plate OCR — **Next**
- watchlist / blacklist / whitelist — **Next**
- plate alias normalization — **Next**
- vehicle-owner linking — **Later**

### Access / Guest Mapping

- allowed zones in profile schema — **Now**
- access schedules in profile schema — **Now**
- rule-based allowed / denied evaluation — **Next**
- hotel guest-room mapping — **Later**
- supplier / contractor access mapping — **Later**
- visitor registration workflows — **Later**

### Model Lifecycle

- schema reserved for model versions — **Now**
- gallery versioning — **Next**
- active / previous model metadata — **Next**
- rollback procedure — **Later**
- fine-tune pipeline — **Later**

## แยกตามสถานที่ใช้งาน

### Home

**Now**
- person at door
- vehicle at gate
- restricted zone movement
- after-hours movement

**Next**
- known resident returned home
- loitering near door
- fall detection
- unknown vehicle alert

**Later**
- elderly inactivity
- child restricted zone specialization
- fence climbing model

### Office

**Now**
- person entry/exit by line
- unknown movement in office zone
- after-hours office presence
- vehicle entered parking

**Next**
- employee attendance with identity
- storage/server room loitering
- asset carried out

**Later**
- tailgating
- policy-based access by staff role

### Hotel

**Now**
- lobby / guest-floor movement detection
- gate vehicle logging
- corridor loitering as generic rule

**Next**
- known guest returned
- unknown person on guest floor
- unregistered visitor alert

**Later**
- room-linked guest mapping
- guest vehicle mapping
- post-checkout movement policy

### Factory

**Now**
- restricted zone intrusion
- warehouse after-hours movement
- loading gate vehicle event logging

**Next**
- contractor schedule logic
- truck gate logs with plate OCR
- unauthorized vehicle alert

**Later**
- PPE detection
- forklift/person proximity
- delivery schedule integration

## ข้อเสนอแนะเชิงปฏิบัติ

ถ้าต้องเริ่มทำจริงในรอบแรก ให้ยึดรายการต่อไปนี้เท่านั้น:

1. person detection
2. vehicle detection
3. zone / line crossing
4. alerts
5. event logs
6. registry schema

จากนั้นค่อยเพิ่ม:

7. face gallery
8. attendance
9. LPR
10. guest/access mapping

## สรุป

matrix นี้ตั้งใจกันไม่ให้โปรเจกต์เริ่มต้นจาก feature list ที่ใหญ่เกินไป โดยบังคับให้ทุกอย่างผ่านคำถามเดียวก่อน:

> ฟีเจอร์นี้ช่วยให้ระบบใช้งานได้จริงเร็วขึ้น หรือแค่ทำให้เอกสารดูใหญ่ขึ้น?

ถ้าเป็นอย่างหลัง ให้เลื่อนไป `Next` หรือ `Later`
