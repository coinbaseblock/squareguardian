"""SQLite-backed storage for face embeddings and person registry."""

import base64
import sqlite3
import time
import uuid
from pathlib import Path
from typing import Optional

import numpy as np

DB_PATH = Path("/data/face.db")


def _connect() -> sqlite3.Connection:
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(DB_PATH))
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    return conn


def _migrate(conn: sqlite3.Connection) -> None:
    """Run schema migrations for new columns."""
    cursor = conn.execute("PRAGMA table_info(persons)")
    existing = {row["name"] for row in cursor.fetchall()}
    new_cols = {
        "first_name": "TEXT DEFAULT ''",
        "last_name": "TEXT DEFAULT ''",
        "car_plate": "TEXT DEFAULT ''",
        "room": "TEXT DEFAULT ''",
        "notes": "TEXT DEFAULT ''",
    }
    for col, typedef in new_cols.items():
        if col not in existing:
            conn.execute(f"ALTER TABLE persons ADD COLUMN {col} {typedef}")
    conn.commit()


def init_db() -> None:
    conn = _connect()
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS persons (
            id          TEXT PRIMARY KEY,
            name        TEXT NOT NULL,
            first_name  TEXT DEFAULT '',
            last_name   TEXT DEFAULT '',
            car_plate   TEXT DEFAULT '',
            room        TEXT DEFAULT '',
            notes       TEXT DEFAULT '',
            source      TEXT DEFAULT 'manual',
            created_at  REAL DEFAULT (strftime('%s','now')),
            updated_at  REAL DEFAULT (strftime('%s','now'))
        );

        CREATE TABLE IF NOT EXISTS face_embeddings (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            person_id   TEXT REFERENCES persons(id) ON DELETE CASCADE,
            embedding   BLOB NOT NULL,
            source      TEXT DEFAULT 'manual',
            quality     REAL,
            created_at  REAL DEFAULT (strftime('%s','now'))
        );

        CREATE TABLE IF NOT EXISTS face_events (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            event_id    TEXT NOT NULL,
            person_id   TEXT REFERENCES persons(id) ON DELETE SET NULL,
            similarity  REAL,
            status      TEXT DEFAULT 'auto',
            created_at  REAL DEFAULT (strftime('%s','now'))
        );

        CREATE TABLE IF NOT EXISTS pending_embeddings (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            person_id   TEXT REFERENCES persons(id) ON DELETE CASCADE,
            embedding   BLOB NOT NULL,
            snapshot    TEXT,
            similarity  REAL,
            quality     REAL,
            camera      TEXT DEFAULT '',
            event_id    TEXT DEFAULT '',
            created_at  REAL DEFAULT (strftime('%s','now'))
        );
    """)
    _migrate(conn)
    conn.commit()
    conn.close()


def register_person(
    name: str,
    embeddings: list[np.ndarray],
    source: str = "manual",
    first_name: str = "",
    last_name: str = "",
    car_plate: str = "",
    room: str = "",
    notes: str = "",
) -> str:
    person_id = f"p-{uuid.uuid4().hex[:8]}"
    conn = _connect()
    conn.execute(
        "INSERT INTO persons (id, name, first_name, last_name, car_plate, room, notes, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
        (person_id, name, first_name, last_name, car_plate, room, notes, source),
    )
    for emb in embeddings:
        conn.execute(
            "INSERT INTO face_embeddings (person_id, embedding, source) VALUES (?, ?, ?)",
            (person_id, emb.astype(np.float32).tobytes(), source),
        )
    conn.commit()
    conn.close()
    return person_id


def update_person(person_id: str, **fields) -> bool:
    """Update person fields (name, first_name, last_name, car_plate, room, notes)."""
    allowed = {"name", "first_name", "last_name", "car_plate", "room", "notes"}
    updates = {k: v for k, v in fields.items() if k in allowed}
    if not updates:
        return False
    updates["updated_at"] = time.time()
    set_clause = ", ".join(f"{k} = ?" for k in updates)
    values = list(updates.values()) + [person_id]
    conn = _connect()
    cur = conn.execute(f"UPDATE persons SET {set_clause} WHERE id = ?", values)
    conn.commit()
    ok = cur.rowcount > 0
    conn.close()
    return ok


def add_embedding(person_id: str, embedding: np.ndarray, source: str = "auto", quality: Optional[float] = None) -> None:
    conn = _connect()
    conn.execute(
        "INSERT INTO face_embeddings (person_id, embedding, source, quality) VALUES (?, ?, ?, ?)",
        (person_id, embedding.astype(np.float32).tobytes(), source, quality),
    )
    conn.execute("UPDATE persons SET updated_at = ? WHERE id = ?", (time.time(), person_id))
    conn.commit()
    conn.close()


def get_all_persons() -> list[dict]:
    conn = _connect()
    rows = conn.execute("""
        SELECT p.id, p.name, p.first_name, p.last_name, p.car_plate, p.room, p.notes,
               p.source, p.created_at,
               COUNT(e.id) as face_count
        FROM persons p
        LEFT JOIN face_embeddings e ON e.person_id = p.id
        GROUP BY p.id
        ORDER BY p.created_at DESC
    """).fetchall()
    conn.close()
    return [dict(r) for r in rows]


def get_all_embeddings() -> list[tuple[str, str, np.ndarray]]:
    """Returns list of (person_id, person_name, embedding)."""
    conn = _connect()
    rows = conn.execute("""
        SELECT p.id, p.name, e.embedding
        FROM face_embeddings e
        JOIN persons p ON p.id = e.person_id
    """).fetchall()
    conn.close()
    result = []
    for r in rows:
        emb = np.frombuffer(r["embedding"], dtype=np.float32).copy()
        result.append((r["id"], r["name"], emb))
    return result


def delete_person(person_id: str) -> bool:
    conn = _connect()
    cur = conn.execute("DELETE FROM persons WHERE id = ?", (person_id,))
    conn.execute("DELETE FROM face_embeddings WHERE person_id = ?", (person_id,))
    conn.commit()
    deleted = cur.rowcount > 0
    conn.close()
    return deleted


def delete_auto_embeddings(person_id: str) -> int:
    """Delete all auto-learned embeddings for a person, keeping manual ones."""
    conn = _connect()
    cur = conn.execute(
        "DELETE FROM face_embeddings WHERE person_id = ? AND source = 'auto'",
        (person_id,),
    )
    conn.execute("UPDATE persons SET updated_at = ? WHERE id = ?", (time.time(), person_id))
    conn.commit()
    deleted = cur.rowcount
    conn.close()
    return deleted


def log_face_event(event_id: str, person_id: Optional[str], similarity: Optional[float], status: str = "auto") -> None:
    conn = _connect()
    conn.execute(
        "INSERT INTO face_events (event_id, person_id, similarity, status) VALUES (?, ?, ?, ?)",
        (event_id, person_id, similarity, status),
    )
    conn.commit()
    conn.close()


# --- Pending embeddings (manual approval) ---

def add_pending_embedding(
    person_id: str,
    embedding: np.ndarray,
    snapshot_b64: str = "",
    similarity: float = 0.0,
    quality: Optional[float] = None,
    camera: str = "",
    event_id: str = "",
) -> int:
    """Store an embedding as pending for user approval."""
    conn = _connect()
    cur = conn.execute(
        "INSERT INTO pending_embeddings (person_id, embedding, snapshot, similarity, quality, camera, event_id) VALUES (?, ?, ?, ?, ?, ?, ?)",
        (person_id, embedding.astype(np.float32).tobytes(), snapshot_b64, similarity, quality, camera, event_id),
    )
    conn.commit()
    pid = cur.lastrowid
    conn.close()
    return pid


def get_pending_embeddings(person_id: Optional[str] = None) -> list[dict]:
    """Get all pending embeddings, optionally filtered by person_id."""
    conn = _connect()
    if person_id:
        rows = conn.execute("""
            SELECT pe.id, pe.person_id, p.name as person_name, pe.snapshot, pe.similarity, pe.quality, pe.camera, pe.event_id, pe.created_at
            FROM pending_embeddings pe
            JOIN persons p ON p.id = pe.person_id
            WHERE pe.person_id = ?
            ORDER BY pe.created_at DESC
        """, (person_id,)).fetchall()
    else:
        rows = conn.execute("""
            SELECT pe.id, pe.person_id, p.name as person_name, pe.snapshot, pe.similarity, pe.quality, pe.camera, pe.event_id, pe.created_at
            FROM pending_embeddings pe
            JOIN persons p ON p.id = pe.person_id
            ORDER BY pe.created_at DESC
        """).fetchall()
    conn.close()
    return [dict(r) for r in rows]


def approve_pending_embedding(pending_id: int) -> bool:
    """Approve a pending embedding: move it to face_embeddings."""
    conn = _connect()
    row = conn.execute("SELECT person_id, embedding, quality FROM pending_embeddings WHERE id = ?", (pending_id,)).fetchone()
    if not row:
        conn.close()
        return False
    conn.execute(
        "INSERT INTO face_embeddings (person_id, embedding, source, quality) VALUES (?, ?, 'approved', ?)",
        (row["person_id"], row["embedding"], row["quality"]),
    )
    conn.execute("DELETE FROM pending_embeddings WHERE id = ?", (pending_id,))
    conn.execute("UPDATE persons SET updated_at = ? WHERE id = ?", (time.time(), row["person_id"]))
    conn.commit()
    conn.close()
    return True


def reject_pending_embedding(pending_id: int) -> bool:
    """Reject and delete a pending embedding."""
    conn = _connect()
    cur = conn.execute("DELETE FROM pending_embeddings WHERE id = ?", (pending_id,))
    conn.commit()
    ok = cur.rowcount > 0
    conn.close()
    return ok


def get_pending_count() -> int:
    """Get total count of pending embeddings."""
    conn = _connect()
    row = conn.execute("SELECT COUNT(*) as cnt FROM pending_embeddings").fetchone()
    conn.close()
    return row["cnt"] if row else 0
