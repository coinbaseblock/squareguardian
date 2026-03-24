"""InsightFace wrapper for face detection and embedding extraction."""

import logging
from typing import Optional

import cv2
import numpy as np
from insightface.app import FaceAnalysis

logger = logging.getLogger(__name__)

_app: Optional[FaceAnalysis] = None


def init_model(model_name: str = "buffalo_s") -> None:
    global _app
    logger.info("Loading InsightFace model: %s", model_name)
    _app = FaceAnalysis(name=model_name, providers=["CPUExecutionProvider"])
    _app.prepare(ctx_id=-1, det_size=(640, 640))
    logger.info("InsightFace model loaded successfully")


def detect_faces(img: np.ndarray) -> list[dict]:
    """Detect faces and return bounding boxes, embeddings, and quality scores."""
    if _app is None:
        raise RuntimeError("Model not initialized")

    faces = _app.get(img)
    results = []
    for face in faces:
        bbox = face.bbox.astype(int).tolist()
        embedding = face.normed_embedding  # already L2-normalized 512-dim
        det_score = float(face.det_score)
        results.append({
            "bbox": bbox,
            "embedding": embedding,
            "confidence": det_score,
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
