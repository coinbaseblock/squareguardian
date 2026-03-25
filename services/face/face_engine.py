"""InsightFace wrapper for face detection and embedding extraction."""

import logging
import os
from typing import Optional

import cv2
import numpy as np
from insightface.app import FaceAnalysis

logger = logging.getLogger(__name__)

_app: Optional[FaceAnalysis] = None

# Detection size: higher values detect smaller/farther faces but use more CPU.
# 640 is standard; 320 is fast but misses small faces.
_DET_SIZE = int(os.getenv("FACE_DET_SIZE", "640"))


def init_model(model_name: str = "buffalo_l") -> None:
    global _app
    logger.info("Loading InsightFace model: %s (det_size=%d)", model_name, _DET_SIZE)
    _app = FaceAnalysis(name=model_name, providers=["CPUExecutionProvider"])
    _app.prepare(ctx_id=-1, det_size=(_DET_SIZE, _DET_SIZE))
    logger.info("InsightFace model loaded successfully")


def _preprocess(img: np.ndarray) -> np.ndarray:
    """Improve image quality for better face detection.

    Applies CLAHE (contrast-limited adaptive histogram equalization)
    to brighten dark/IR camera frames without blowing out highlights.
    """
    # Only apply CLAHE if image is relatively dark (mean luminance < 100)
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    mean_lum = gray.mean()
    if mean_lum < 100:
        lab = cv2.cvtColor(img, cv2.COLOR_BGR2LAB)
        l, a, b = cv2.split(lab)
        clahe = cv2.createCLAHE(clipLimit=3.0, tileGridSize=(8, 8))
        l = clahe.apply(l)
        img = cv2.cvtColor(cv2.merge([l, a, b]), cv2.COLOR_LAB2BGR)
        logger.debug("CLAHE applied (mean_lum=%.0f)", mean_lum)

    return img


def detect_faces(img: np.ndarray) -> list[dict]:
    """Detect faces and return bounding boxes, embeddings, and quality scores.

    For high-resolution images (>1080p), runs detection at multiple scales
    to catch both close-up and distant faces.
    """
    if _app is None:
        raise RuntimeError("Model not initialized")

    img = _preprocess(img)

    h, w = img.shape[:2]

    # For high-res images, try detection at original size first for distant faces,
    # then also at scaled-down version. Merge results by NMS.
    faces = _app.get(img)

    # If no faces found on high-res image and image is large, try with
    # increased det_size temporarily for better small-face detection.
    if not faces and max(h, w) >= 1920:
        logger.debug("No faces at default det_size, retrying with larger det_size on %dx%d image", w, h)
        _app.prepare(ctx_id=-1, det_size=(960, 960))
        faces = _app.get(img)
        # Restore original det_size
        _app.prepare(ctx_id=-1, det_size=(_DET_SIZE, _DET_SIZE))

    results = []
    for face in faces:
        bbox = face.bbox.astype(int).tolist()
        embedding = face.normed_embedding  # already L2-normalized 512-dim
        det_score = float(face.det_score)

        # Compute face quality: consider detection score + face size relative to image
        face_w = bbox[2] - bbox[0]
        face_h = bbox[3] - bbox[1]
        face_area_ratio = (face_w * face_h) / (w * h) if w * h > 0 else 0

        results.append({
            "bbox": bbox,
            "embedding": embedding,
            "confidence": det_score,
            "face_area_ratio": face_area_ratio,
        })
    return results


def compute_similarity(emb1: np.ndarray, emb2: np.ndarray) -> float:
    """Compute cosine similarity between two normalized embeddings."""
    return float(np.dot(emb1, emb2))


def identify(
    query_embedding: np.ndarray,
    known_embeddings: list[tuple[str, str, np.ndarray]],
    match_threshold: float = 0.55,
    suggest_threshold: float = 0.45,
) -> Optional[dict]:
    """Match a face embedding against known persons.

    Args:
        query_embedding: 512-dim normalized face embedding
        known_embeddings: list of (person_id, person_name, embedding)
        match_threshold: similarity above this → auto-match
        suggest_threshold: similarity above this → suggest

    Returns:
        dict with person_id, name, similarity, status or None
    """
    if not known_embeddings:
        return None

    best_sim = -1.0
    best_person_id = ""
    best_name = ""

    # Group embeddings by person and take the best match per person
    person_best: dict[str, tuple[str, float]] = {}
    for pid, name, emb in known_embeddings:
        sim = compute_similarity(query_embedding, emb)
        if pid not in person_best or sim > person_best[pid][1]:
            person_best[pid] = (name, sim)

    for pid, (name, sim) in person_best.items():
        if sim > best_sim:
            best_sim = sim
            best_person_id = pid
            best_name = name

    if best_sim >= match_threshold:
        return {
            "person_id": best_person_id,
            "name": best_name,
            "similarity": round(best_sim, 4),
            "status": "match",
        }
    elif best_sim >= suggest_threshold:
        return {
            "person_id": best_person_id,
            "name": best_name,
            "similarity": round(best_sim, 4),
            "status": "suggest",
        }
    return None
