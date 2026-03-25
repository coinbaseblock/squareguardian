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
    Also applies mild denoising for security camera footage which tends
    to have compression artifacts.
    """
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    mean_lum = gray.mean()

    # Apply CLAHE for dark images OR low-contrast images
    std_lum = gray.std()
    if mean_lum < 120 or std_lum < 40:
        lab = cv2.cvtColor(img, cv2.COLOR_BGR2LAB)
        l, a, b = cv2.split(lab)
        clip_limit = 3.0 if mean_lum < 80 else 2.0
        clahe = cv2.createCLAHE(clipLimit=clip_limit, tileGridSize=(8, 8))
        l = clahe.apply(l)
        img = cv2.cvtColor(cv2.merge([l, a, b]), cv2.COLOR_LAB2BGR)
        logger.debug("CLAHE applied (mean_lum=%.0f, std=%.0f)", mean_lum, std_lum)

    # Light denoising to reduce JPEG/compression artifacts from security cameras.
    # fastNlMeansDenoisingColored is effective but expensive; use small h values.
    img_h, img_w = img.shape[:2]
    if max(img_h, img_w) <= 1920:
        img = cv2.fastNlMeansDenoisingColored(img, None, h=3, hColor=3,
                                               templateWindowSize=7, searchWindowSize=21)

    return img


def _add_border_padding(img: np.ndarray, pad_pct: float = 0.10) -> tuple[np.ndarray, int, int]:
    """Add border padding around image to help detect faces near frame edges.

    Wide-angle/fisheye cameras often place people at the edges where they get
    cut off. Adding padding gives the face detector more context.

    Returns (padded_image, pad_x, pad_y) so bboxes can be offset back.
    """
    h, w = img.shape[:2]
    pad_x = int(w * pad_pct)
    pad_y = int(h * pad_pct)
    padded = cv2.copyMakeBorder(img, pad_y, pad_y, pad_x, pad_x,
                                cv2.BORDER_REFLECT_101)
    return padded, pad_x, pad_y


def detect_faces(img: np.ndarray) -> list[dict]:
    """Detect faces and return bounding boxes, embeddings, and quality scores.

    Uses multi-strategy detection:
    1. Standard detection on preprocessed image
    2. Padded detection for faces near frame edges (wide-angle cameras)
    3. Upscaled detection with larger det_size for small/distant faces
    4. Center crop for wide-angle security cameras where face is small in frame
    """
    if _app is None:
        raise RuntimeError("Model not initialized")

    img = _preprocess(img)

    h, w = img.shape[:2]

    # Strategy 1: Standard detection
    faces = _app.get(img)

    # Strategy 2: If no faces found, try with border padding.
    # This helps when the face is at the very edge of the frame (common with
    # wide-angle security cameras like Tapo). The padding gives the detector
    # context around the face that would otherwise be clipped.
    if not faces:
        padded, pad_x, pad_y = _add_border_padding(img, pad_pct=0.12)
        logger.debug("No faces at standard, retrying with border padding on %dx%d image", w, h)
        padded_faces = _app.get(padded)
        if padded_faces:
            # Adjust bounding boxes back to original image coordinates
            for face in padded_faces:
                face.bbox[0] -= pad_x
                face.bbox[1] -= pad_y
                face.bbox[2] -= pad_x
                face.bbox[3] -= pad_y
                # Clamp to original image bounds
                face.bbox[0] = max(0, face.bbox[0])
                face.bbox[1] = max(0, face.bbox[1])
                face.bbox[2] = min(w, face.bbox[2])
                face.bbox[3] = min(h, face.bbox[3])
            faces = padded_faces
            logger.info("Detected %d face(s) with border padding", len(faces))

    # Strategy 3: If still no faces, try larger det_size.
    # Previously only triggered for ≥1920px images, but small faces in 720p
    # camera snapshots also benefit from a larger detection grid.
    if not faces and _DET_SIZE < 960:
        logger.debug("No faces at default det_size, retrying with larger det_size on %dx%d image", w, h)
        _app.prepare(ctx_id=-1, det_size=(960, 960))
        faces = _app.get(img)
        # Restore original det_size
        _app.prepare(ctx_id=-1, det_size=(_DET_SIZE, _DET_SIZE))

    # Strategy 4: Center crop — for security cameras where the subject stands
    # in front of the camera but the wide-angle lens makes the face small.
    # Cropping to the central region effectively enlarges the face for detection.
    if not faces:
        crop_pct = 0.35  # crop 35% from each side → keep central 30% width
        cx1 = int(w * crop_pct)
        cy1 = int(h * 0.10)  # less top crop since security cameras are elevated
        cx2 = int(w * (1 - crop_pct))
        cy2 = int(h * 0.85)
        if cx2 - cx1 > 100 and cy2 - cy1 > 100:
            crop = img[cy1:cy2, cx1:cx2]
            logger.debug("No faces, retrying with center crop (%d,%d)-(%d,%d) on %dx%d image",
                         cx1, cy1, cx2, cy2, w, h)
            crop_faces = _app.get(crop)
            if crop_faces:
                # Adjust bounding boxes back to original image coordinates
                for face in crop_faces:
                    face.bbox[0] += cx1
                    face.bbox[1] += cy1
                    face.bbox[2] += cx1
                    face.bbox[3] += cy1
                faces = crop_faces
                logger.info("Detected %d face(s) with center crop", len(faces))

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
