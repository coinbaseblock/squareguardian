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

class RegisterRequest(BaseModel):
    name: str
    images: list[str]  # list of base64 images

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
                # Auto-learn: if high-confidence match, add embedding to improve accuracy
                if result["status"] == "match" and result["similarity"] >= 0.65:
                    store.add_embedding(
                        result["person_id"],
                        face["embedding"],
                        source="auto",
                        quality=face["confidence"],
                    )

    return {"matches": matches, "faces_detected": len(faces), "has_unknown": len(faces) > len(matches)}


@app.post("/api/face/register")
def register(req: RegisterRequest):
    if not req.name.strip():
        raise HTTPException(400, "name is required")
    if not req.images:
        raise HTTPException(400, "at least one image is required")

    embeddings = []
    for b64 in req.images:
        img = decode_image(b64)
        faces = face_engine.detect_faces(img)
        if not faces:
            continue
        # Take the face with highest confidence
        best = max(faces, key=lambda f: f["confidence"])
        embeddings.append(best["embedding"])

    if not embeddings:
        raise HTTPException(400, "no faces detected in provided images")

    person_id = store.register_person(req.name.strip(), embeddings)
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
