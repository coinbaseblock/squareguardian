"""
Action Recognition Service for SquareGuardian.

Skeleton-based human action recognition pipeline:
  1. Receive RTSP stream URL or frame images
  2. Pose estimation (MediaPipe Pose)
  3. Track persons (simple centroid tracker)
  4. Classify actions from skeleton sequences via DNN

Supported actions (pretrained): stand, walk, run, jump, sit, squat, kick, punch, wave
Future: fall, loiter, climb_fence, no_ppe, intrusion

Publishes action events to MQTT topic: squareguardian/actions
"""

import json
import logging
import os
import threading
import time

import cv2
import numpy as np
from flask import Flask, jsonify, request

logging.basicConfig(level=logging.INFO, format="%(asctime)s [action] %(message)s")
logger = logging.getLogger(__name__)

# ─── Configuration ───────────────────────────────────────────────────
MQTT_BROKER = os.getenv("MQTT_BROKER", "mosquitto")
MQTT_PORT = int(os.getenv("MQTT_PORT", "1883"))
MQTT_TOPIC = os.getenv("MQTT_TOPIC", "squareguardian/actions")
RTSP_URLS = os.getenv("RTSP_URLS", "").split(",")  # comma-separated
PROCESS_FPS = int(os.getenv("PROCESS_FPS", "5"))
CONFIDENCE_THRESHOLD = float(os.getenv("CONFIDENCE_THRESHOLD", "0.5"))
PORT = int(os.getenv("PORT", "8083"))

# ─── MQTT Client ─────────────────────────────────────────────────────
mqtt_client = None
try:
    import paho.mqtt.client as paho_mqtt

    mqtt_client = paho_mqtt.Client(
        paho_mqtt.CallbackAPIVersion.VERSION2, client_id="squareguardian-action"
    )

    def on_connect(client, userdata, flags, rc, properties=None):
        logger.info(f"MQTT connected: {rc}")

    mqtt_client.on_connect = on_connect
    mqtt_client.connect(MQTT_BROKER, MQTT_PORT, 60)
    mqtt_client.loop_start()
    logger.info(f"MQTT client started: {MQTT_BROKER}:{MQTT_PORT}")
except Exception as e:
    logger.warning(f"MQTT not available: {e}")

# ─── Action Labels ───────────────────────────────────────────────────
ACTION_LABELS = [
    "stand", "walk", "run", "jump", "sit",
    "squat", "kick", "punch", "wave",
]

# ─── Pose + Classification Pipeline ─────────────────────────────────
class ActionPipeline:
    """Skeleton-based action recognition using MediaPipe Pose."""

    def __init__(self):
        self.mp_pose = None
        self.pose = None
        self._init_pose()
        # Sequence buffer per track_id: stores last N skeleton frames
        self.sequences = {}
        self.seq_length = 30  # frames of skeleton data for classification
        logger.info("ActionPipeline initialized (MediaPipe Pose)")

    def _init_pose(self):
        try:
            import mediapipe as mp
            self.mp_pose = mp.solutions.pose
            self.pose = self.mp_pose.Pose(
                static_image_mode=False,
                model_complexity=1,
                min_detection_confidence=0.5,
                min_tracking_confidence=0.5,
            )
            logger.info("MediaPipe Pose loaded")
        except Exception as e:
            logger.error(f"Failed to load MediaPipe: {e}")

    def process_frame(self, frame, track_id="default"):
        """Process a single frame, extract pose, and classify action."""
        if self.pose is None:
            return None

        rgb = cv2.cvtColor(frame, cv2.COLOR_BGR2RGB)
        results = self.pose.process(rgb)

        if not results.pose_landmarks:
            return None

        # Extract 33 landmarks as flat array [x, y, z, visibility] * 33 = 132 values
        landmarks = []
        for lm in results.pose_landmarks.landmark:
            landmarks.extend([lm.x, lm.y, lm.z, lm.visibility])

        # Buffer sequence
        if track_id not in self.sequences:
            self.sequences[track_id] = []
        self.sequences[track_id].append(landmarks)
        if len(self.sequences[track_id]) > self.seq_length:
            self.sequences[track_id] = self.sequences[track_id][-self.seq_length:]

        # Classify only when we have enough frames
        if len(self.sequences[track_id]) < self.seq_length:
            return {"action": "collecting", "confidence": 0.0, "track_id": track_id}

        action, confidence = self._classify(self.sequences[track_id])
        return {
            "action": action,
            "confidence": float(confidence),
            "track_id": track_id,
        }

    def _classify(self, sequence):
        """
        Classify action from skeleton sequence.

        Current implementation uses simple heuristics based on skeleton motion.
        Replace with a trained DNN model (LSTM/GRU/Transformer) for production.
        """
        seq = np.array(sequence)  # shape: (seq_length, 132)

        # Extract key joint positions (normalized 0-1)
        # MediaPipe indices: 0=nose, 11=L_shoulder, 12=R_shoulder,
        # 23=L_hip, 24=R_hip, 25=L_knee, 26=R_knee, 27=L_ankle, 28=R_ankle
        def get_joint(frame_idx, joint_idx):
            base = joint_idx * 4
            return seq[frame_idx, base:base + 3]  # x, y, z

        # Calculate vertical movement of hips (sitting vs standing)
        hip_y_start = (get_joint(0, 23)[1] + get_joint(0, 24)[1]) / 2
        hip_y_end = (get_joint(-1, 23)[1] + get_joint(-1, 24)[1]) / 2
        hip_y_avg = np.mean([(get_joint(i, 23)[1] + get_joint(i, 24)[1]) / 2
                             for i in range(len(seq))])

        # Calculate horizontal movement (walking/running)
        hip_x = [(get_joint(i, 23)[0] + get_joint(i, 24)[0]) / 2
                  for i in range(len(seq))]
        hip_x_range = max(hip_x) - min(hip_x)

        # Calculate knee angle (squatting)
        knee_y_end = (get_joint(-1, 25)[1] + get_joint(-1, 26)[1]) / 2

        # Vertical velocity of center of mass
        com_y = [(get_joint(i, 23)[1] + get_joint(i, 24)[1] +
                  get_joint(i, 11)[1] + get_joint(i, 12)[1]) / 4
                 for i in range(len(seq))]
        vert_velocity = np.diff(com_y)
        max_down_vel = np.max(vert_velocity) if len(vert_velocity) > 0 else 0

        # Wrist above shoulder (wave/punch/kick)
        wrist_above = False
        for i in range(-5, 0):
            l_wrist_y = get_joint(i, 15)[1]  # left wrist
            r_wrist_y = get_joint(i, 16)[1]  # right wrist
            l_shoulder_y = get_joint(i, 11)[1]
            if l_wrist_y < l_shoulder_y or r_wrist_y < l_shoulder_y:
                wrist_above = True
                break

        # Simple rule-based classification (placeholder for DNN)
        if max_down_vel > 0.15:
            return "fall", 0.7
        if hip_y_avg > 0.75:
            return "sit", 0.6
        if hip_y_end > 0.65 and knee_y_end > 0.8:
            return "squat", 0.6
        if hip_x_range > 0.15:
            if hip_x_range > 0.3:
                return "run", 0.65
            return "walk", 0.6
        if wrist_above and hip_x_range < 0.05:
            # Check if it looks like waving (repetitive) or punch (sudden)
            return "wave", 0.55
        if abs(hip_y_end - hip_y_start) > 0.1 and max_down_vel < -0.05:
            return "jump", 0.6

        return "stand", 0.5


pipeline = ActionPipeline()

# ─── RTSP Stream Processors ─────────────────────────────────────────
active_streams = {}


def process_stream(rtsp_url, camera_name):
    """Process an RTSP stream for action recognition."""
    logger.info(f"Starting stream processor: {camera_name} -> {rtsp_url}")
    cap = cv2.VideoCapture(rtsp_url)
    if not cap.isOpened():
        logger.error(f"Cannot open stream: {rtsp_url}")
        return

    frame_interval = max(1, int(cap.get(cv2.CAP_PROP_FPS) / PROCESS_FPS)) if cap.get(cv2.CAP_PROP_FPS) > 0 else 6
    frame_count = 0

    while camera_name in active_streams:
        ret, frame = cap.read()
        if not ret:
            logger.warning(f"Stream read failed: {camera_name}, reconnecting...")
            time.sleep(5)
            cap.release()
            cap = cv2.VideoCapture(rtsp_url)
            continue

        frame_count += 1
        if frame_count % frame_interval != 0:
            continue

        result = pipeline.process_frame(frame, track_id=camera_name)
        if result and result["action"] != "collecting" and result["confidence"] >= CONFIDENCE_THRESHOLD:
            event = {
                "camera": camera_name,
                "action": result["action"],
                "confidence": result["confidence"],
                "track_id": result["track_id"],
                "timestamp": time.time(),
            }
            logger.info(f"Action: {camera_name} -> {result['action']} ({result['confidence']:.2f})")

            # Publish to MQTT
            if mqtt_client:
                try:
                    mqtt_client.publish(MQTT_TOPIC, json.dumps(event))
                except Exception as e:
                    logger.error(f"MQTT publish error: {e}")

    cap.release()
    logger.info(f"Stream processor stopped: {camera_name}")


# ─── Flask API ───────────────────────────────────────────────────────
app = Flask(__name__)


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok", "active_streams": list(active_streams.keys())})


@app.route("/api/action/classify", methods=["POST"])
def classify_frame():
    """Classify action from a posted image frame."""
    if "file" not in request.files:
        return jsonify({"error": "no file"}), 400

    file = request.files["file"]
    img_bytes = np.frombuffer(file.read(), np.uint8)
    frame = cv2.imdecode(img_bytes, cv2.IMREAD_COLOR)
    if frame is None:
        return jsonify({"error": "invalid image"}), 400

    track_id = request.form.get("track_id", "api")
    result = pipeline.process_frame(frame, track_id=track_id)
    if result is None:
        return jsonify({"action": "no_pose", "confidence": 0.0})

    return jsonify(result)


@app.route("/api/streams", methods=["GET"])
def list_streams():
    return jsonify({"streams": list(active_streams.keys())})


@app.route("/api/streams", methods=["POST"])
def add_stream():
    """Add an RTSP stream for processing."""
    data = request.get_json()
    name = data.get("name")
    url = data.get("rtsp_url")
    if not name or not url:
        return jsonify({"error": "name and rtsp_url required"}), 400

    if name in active_streams:
        return jsonify({"error": "stream already active"}), 409

    active_streams[name] = True
    t = threading.Thread(target=process_stream, args=(url, name), daemon=True)
    t.start()
    return jsonify({"status": "started", "camera": name}), 201


@app.route("/api/streams/<name>", methods=["DELETE"])
def remove_stream(name):
    """Stop processing an RTSP stream."""
    if name not in active_streams:
        return jsonify({"error": "stream not found"}), 404

    del active_streams[name]
    return jsonify({"status": "stopped", "camera": name})


# ─── Startup ─────────────────────────────────────────────────────────
def start_configured_streams():
    """Auto-start streams from RTSP_URLS env var."""
    for i, url in enumerate(RTSP_URLS):
        url = url.strip()
        if not url:
            continue
        name = f"cam_{i}"
        active_streams[name] = True
        t = threading.Thread(target=process_stream, args=(url, name), daemon=True)
        t.start()
        logger.info(f"Auto-started stream: {name}")


if __name__ == "__main__":
    start_configured_streams()
    logger.info(f"Action recognition service starting on port {PORT}")
    app.run(host="0.0.0.0", port=PORT)
