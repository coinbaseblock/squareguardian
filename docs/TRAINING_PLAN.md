# Training Plan: Re-identification Model

## Overview

ใช้ข้อมูลที่ผู้ใช้จัดกลุ่ม (Grouped Events) เพื่อสอน model ให้แยกแยะบุคคลหรือรถแต่ละคัน/คนได้อัตโนมัติ

---

## ขั้นตอนการทำงาน (Step-by-Step)

### 1. เก็บข้อมูล (Data Collection)

- ผู้ใช้เปิดหน้า `/events` แล้วกดปุ่ม **"เลือกหลายรายการ"**
- เลือกรูปหลายๆ รูปที่เป็น **คนเดียวกัน** หรือ **รถคันเดียวกัน** (มุมต่างๆ, เวลาต่างกัน)
- ตั้งชื่อกลุ่ม เช่น "สมชาย", "รถขาว Toyota A0213"
- ระบบบันทึกเป็น **group** พร้อม thumbnail ทุกรูปที่เลือก

### 2. Export Training Data

- เรียก API: `GET /api/training-data`
- ได้ JSON ที่รวมข้อมูลกลุ่มทั้งหมด พร้อม thumbnail (base64)
- แต่ละกลุ่ม = 1 identity (คนเดียวกัน / รถคันเดียวกัน)

**ตัวอย่าง Output:**
```json
{
  "count": 3,
  "training_data": [
    {
      "group_id": "g-123",
      "group_name": "สมชาย",
      "label": "person",
      "events": [
        {"id": "ev1", "thumbnail": "base64...", ...},
        {"id": "ev2", "thumbnail": "base64...", ...}
      ]
    }
  ]
}
```

### 3. เตรียม Dataset

จาก training data ที่ได้ ให้แปลงเป็นโครงสร้างสำหรับ model:

```
training_data/
├── person/
│   ├── สมชาย/
│   │   ├── ev1.jpg
│   │   ├── ev2.jpg
│   │   └── ev3.jpg
│   ├── คนส่งพัสดุ/
│   │   ├── ev4.jpg
│   │   └── ev5.jpg
├── car/
│   ├── รถขาว_Toyota/
│   │   ├── ev6.jpg
│   │   └── ev7.jpg
```

**Script ตัวอย่าง:**
```python
import json, base64, os

data = requests.get("http://localhost:8080/api/training-data").json()
for entry in data["training_data"]:
    folder = f"training_data/{entry['label']}/{entry['group_name']}"
    os.makedirs(folder, exist_ok=True)
    for ev in entry["events"]:
        if ev.get("thumbnail"):
            img = base64.b64decode(ev["thumbnail"])
            with open(f"{folder}/{ev['id']}.jpg", "wb") as f:
                f.write(img)
```

### 4. เลือก Model Architecture

#### สำหรับ Person Re-identification:
- **แนะนำ:** Siamese Network หรือ Triplet Loss Network
- **Backbone:** MobileNet V2 หรือ ResNet-18 (เบาพอสำหรับ edge device)
- **Framework:** PyTorch หรือ TensorFlow Lite

#### สำหรับ Vehicle Re-identification:
- **แนะนำ:** เหมือนกัน แต่ input เป็นรูปรถ
- ใช้ feature extraction แล้ว compare ด้วย cosine similarity

### 5. Training Process

```
┌──────────────┐     ┌───────────────┐     ┌──────────────┐
│  Grouped     │     │  Feature      │     │  Embedding   │
│  Images      │────▶│  Extractor    │────▶│  Vector      │
│  (จากกลุ่ม)  │     │  (CNN)        │     │  (128-dim)   │
└──────────────┘     └───────────────┘     └──────────────┘
                                                  │
                                           ┌──────▼──────┐
                                           │  Triplet    │
                                           │  Loss       │
                                           │  Training   │
                                           └─────────────┘
```

**Triplet Loss:**
- **Anchor:** รูปจากกลุ่ม A (เช่น สมชาย มุมตรง)
- **Positive:** รูปอื่นจากกลุ่ม A (เช่น สมชาย มุมข้าง)
- **Negative:** รูปจากกลุ่ม B (เช่น คนส่งของ)
- Model เรียนรู้ว่า Anchor-Positive ใกล้กัน, Anchor-Negative ไกลกัน

### 6. Deploy & Inference

1. Export model เป็น ONNX หรือ TFLite
2. เพิ่ม inference endpoint ใน SpaceGuardian
3. เมื่อ event ใหม่เข้ามา:
   - Extract embedding vector จากรูป
   - Compare กับ embedding ของกลุ่มที่รู้จัก
   - ถ้า cosine similarity > threshold → auto-assign identity

```
Event ใหม่ → Extract Embedding → Compare กับ Known Embeddings
                                         │
                                    ┌────▼────┐
                                    │ > 0.85  │──▶ "สมชาย" (auto-assign)
                                    │ > 0.70  │──▶ "อาจเป็นสมชาย" (suggest)
                                    │ < 0.70  │──▶ "ไม่รู้จัก" (manual)
                                    └─────────┘
```

### 7. Continuous Learning

- เมื่อผู้ใช้ยืนยัน/แก้ไข identity → เพิ่มเข้ากลุ่ม → re-train
- เก็บ embedding ของแต่ละกลุ่มเป็น running average
- ทุกๆ 100 events ใหม่ → trigger re-training รอบใหม่

---

## ข้อกำหนดขั้นต่ำ

| รายการ | ขั้นต่ำ | แนะนำ |
|--------|---------|-------|
| จำนวนรูปต่อกลุ่ม | 5 | 20+ |
| จำนวนกลุ่ม (identities) | 3 | 10+ |
| GPU สำหรับ training | ไม่จำเป็น (CPU ได้) | GPU จะเร็วกว่า 10x |
| เวลา training | ~10 นาที (CPU) | ~1 นาที (GPU) |

---

## API Endpoints ที่เกี่ยวข้อง

| Endpoint | Method | คำอธิบาย |
|----------|--------|----------|
| `/api/group` | POST | สร้างกลุ่มใหม่ (name, label, event_ids) |
| `/api/groups` | GET | ดูกลุ่มทั้งหมด |
| `/api/groups/delete` | POST | ลบกลุ่ม |
| `/api/training-data` | GET | Export ข้อมูลสำหรับ training |
