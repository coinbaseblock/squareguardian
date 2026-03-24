"""SQLite-backed storage for face embeddings and person registry."""

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


def init_db() -> None:
    conn = _connect()
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS persons (
            id          TEXT PRIMARY KEY,
            name        TEXT NOT NULL,
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
    """)
    conn.commit()
    conn.close()


def register_person(name: str, embeddings: list[np.ndarray], source: str = "manual") -> str:
    person_id = f"p-{uuid.uuid4().hex[:8]}"
    conn = _connect()
    conn.execute(
        "INSERT INTO persons (id, name, source) VALUES (?, ?, ?)",
        (person_id, name, source),
    )
    for emb in embeddings:
        conn.execute(
            "INSERT INTO face_embeddings (person_id, embedding, source) VALUES (?, ?, ?)",
            (person_id, emb.astype(np.float32).tobytes(), source),
        )
    conn.commit()
    conn.close()
    return person_id


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
        SELECT p.id, p.name, p.source, p.created_at,
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
