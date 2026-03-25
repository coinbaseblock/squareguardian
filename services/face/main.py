"""SquareGuardian Face Service — local face recognition via InsightFace."""

import base64
import io
import logging
import os
from contextlib import asynccontextmanager

import cv2
import numpy as np
from fastapi import FastAPI, HTTPException
from PIL import Image
from pydantic import BaseModel

import embedding_store as store
import face_engine

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

MODEL_NAME = os.getenv("MODEL_NAME", "buffalo_s")
MATCH_THRESHOLD = float(os.getenv("MATCH_THRESHOLD", "0.55"))
SUGGEST_THRESHOLD = float(os.getenv("SUGGEST_THRESHOLD", "0.45"))


@asynccontextmanager
async def lifespan(app: FastAPI):
    store.init_db()
    face_engine.init_model(MODEL_NAME)
    logger.info("Face service ready (model=%s, match=%.2f, suggest=%.2f)",
                MODEL_NAME, MATCH_THRESHOLD, SUGGEST_THRESHOLD)
    yield


app = FastAPI(title="SquareGuardian Face Service", lifespan=lifespan)


def decode_image(b64: str) -> np.ndarray:
    """Decode base64 image to BGR numpy array."""
    data = base64.b64decode(b64)
    img = Image.open(io.BytesIO(data)).convert("RGB")
    return cv2.cvtColor(np.array(img), cv2.COLOR_RGB2BGR)


# --- Models ---

class DetectRequest(BaseModel):
    image: str  # base64 encoded

class IdentifyRequest(BaseModel):
    image: str  # base64 encoded
    event_id: str = ""  # optional frigate event id
    camera: str = ""  # camera name for pending embedding context

class RegisterRequest(BaseModel):
    name: str
    images: list[str]  # list of base64 images
    first_name: str = ""
    last_name: str = ""
    car_plate: str = ""
    room: str = ""
    notes: str = ""

class UpdatePersonRequest(BaseModel):
    name: str = ""
    first_name: str = ""
    last_name: str = ""
    car_plate: str = ""
    room: str = ""
    notes: str = ""

class CompareRequest(BaseModel):
    image1: str
    image2: str


# --- Endpoints ---

@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/api/face/detect")
def detect(req: DetectRequest):
    img = decode_image(req.image)
    faces = face_engine.detect_faces(img)
    return {
        "faces": [
            {
                "bbox": f["bbox"],
                "confidence": f["confidence"],
            }
            for f in faces
        ],
        "count": len(faces),
    }


@app.post("/api/face/identify")
def identify(req: IdentifyRequest):
    img = decode_image(req.image)
    faces = face_engine.detect_faces(img)

    if not faces:
        return {"matches": [], "faces_detected": 0}

    known = store.get_all_embeddings()
    matches = []

    for face in faces:
        result = face_engine.identify(
            face["embedding"], known,
            match_threshold=MATCH_THRESHOLD,
            suggest_threshold=SUGGEST_THRESHOLD,
        )
        if result:
            matches.append(result)
            # Log the face event
            if req.event_id:
                store.log_face_event(
                    req.event_id,
                    result["person_id"],
                    result["similarity"],
                    result["status"],
                )
                # Save as pending for user approval instead of auto-learning
                if result["status"] == "match" and result["similarity"] >= 0.65:
                    # Crop face from image and encode as base64 snapshot
                    snapshot_b64 = ""
                    try:
                        bbox = face.get("bbox")
                        if bbox:
                            x1, y1, x2, y2 = [int(v) for v in bbox]
                            h, w = img.shape[:2]
                            # Add padding around face
                            pad = int(max(x2 - x1, y2 - y1) * 0.3)
                            x1, y1 = max(0, x1 - pad), max(0, y1 - pad)
                            x2, y2 = min(w, x2 + pad), min(h, y2 + pad)
                            crop = img[y1:y2, x1:x2]
                            _, buf = cv2.imencode(".jpg", crop, [cv2.IMWRITE_JPEG_QUALITY, 80])
                            snapshot_b64 = base64.b64encode(buf).decode()
                    except Exception:
                        pass
                    store.add_pending_embedding(
                        result["person_id"],
                        face["embedding"],
                        snapshot_b64=snapshot_b64,
                        similarity=result["similarity"],
                        quality=face["confidence"],
                        camera=req.camera,
                        event_id=req.event_id,
                    )

    return {"matches": matches, "faces_detected": len(faces), "has_unknown": len(faces) > len(matches)}


@app.post("/api/face/register")
def register(req: RegisterRequest):
    if not req.name.strip():
        raise HTTPException(400, "name is required")
    if not req.images:
        raise HTTPException(400, "at least one image is required")

    embeddings = []
    for i, b64 in enumerate(req.images):
        try:
            img = decode_image(b64)
        except Exception as e:
            logger.warning("Failed to decode image %d: %s", i, e)
            continue
        try:
            faces = face_engine.detect_faces(img)
        except Exception as e:
            logger.error("Face detection failed on image %d: %s", i, e)
            continue
        if not faces:
            continue
        # Take the face with highest confidence
        best = max(faces, key=lambda f: f["confidence"])
        embeddings.append(best["embedding"])

    if not embeddings:
        raise HTTPException(400, "no faces detected in provided images")

    person_id = store.register_person(
        req.name.strip(), embeddings,
        first_name=req.first_name.strip(),
        last_name=req.last_name.strip(),
        car_plate=req.car_plate.strip(),
        room=req.room.strip(),
        notes=req.notes.strip(),
    )
    return {
        "status": "registered",
        "person_id": person_id,
        "name": req.name.strip(),
        "embeddings_count": len(embeddings),
    }


@app.get("/api/face/gallery")
def gallery():
    persons = store.get_all_persons()
    return {"persons": persons}


@app.delete("/api/face/gallery/{person_id}")
def delete_person(person_id: str):
    if store.delete_person(person_id):
        return {"status": "deleted"}
    raise HTTPException(404, "person not found")


@app.delete("/api/face/gallery/{person_id}/auto-embeddings")
def delete_auto_embeddings(person_id: str):
    """Delete all auto-learned embeddings for a person (keep manual ones)."""
    count = store.delete_auto_embeddings(person_id)
    return {"status": "deleted", "deleted_count": count}


@app.put("/api/face/gallery/{person_id}")
def update_person(person_id: str, req: UpdatePersonRequest):
    fields = {k: v.strip() for k, v in req.model_dump().items() if v.strip()}
    if not fields:
        raise HTTPException(400, "no fields to update")
    if store.update_person(person_id, **fields):
        return {"status": "updated"}
    raise HTTPException(404, "person not found")


@app.get("/api/face/pending")
def get_pending(person_id: str = ""):
    items = store.get_pending_embeddings(person_id or None)
    return {"pending": items, "count": len(items)}


@app.post("/api/face/pending/{pending_id}/approve")
def approve_pending(pending_id: int):
    if store.approve_pending_embedding(pending_id):
        return {"status": "approved"}
    raise HTTPException(404, "pending embedding not found")


@app.post("/api/face/pending/{pending_id}/reject")
def reject_pending(pending_id: int):
    if store.reject_pending_embedding(pending_id):
        return {"status": "rejected"}
    raise HTTPException(404, "pending embedding not found")


@app.post("/api/face/compare")
def compare(req: CompareRequest):
    img1 = decode_image(req.image1)
    img2 = decode_image(req.image2)

    faces1 = face_engine.detect_faces(img1)
    faces2 = face_engine.detect_faces(img2)

    if not faces1 or not faces2:
        raise HTTPException(400, "no face detected in one or both images")

    sim = face_engine.compute_similarity(faces1[0]["embedding"], faces2[0]["embedding"])
    return {
        "similarity": round(sim, 4),
        "same_person": sim >= MATCH_THRESHOLD,
    }
