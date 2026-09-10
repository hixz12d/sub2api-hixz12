"""Local benchmark release catalog; production workers must share a central authority.

SQLite serializes administrative transitions. This module neither downloads releases
nor starts evaluations. Released bytes and validation receipts are retained.
"""
from contextlib import contextmanager
import hashlib
import json
import sqlite3


class CatalogError(ValueError):
    pass


def encode(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def label(value):
    if not isinstance(value, str) or not value.strip() or value != value.strip() or len(value) > 256 or any(ord(c) < 32 for c in value):
        raise CatalogError("invalid_label")
    return value


class BenchmarkCatalog:
    def __init__(self, path):
        self.db = sqlite3.connect(path, timeout=10, isolation_level=None)
        self.db.execute("PRAGMA foreign_keys=ON")
        self.db.executescript("""
CREATE TABLE IF NOT EXISTS packages(
 id TEXT NOT NULL, version TEXT NOT NULL, digest TEXT NOT NULL,
 manifest TEXT NOT NULL, payload BLOB NOT NULL,
 PRIMARY KEY(id, version), UNIQUE(id, version, digest));
CREATE TABLE IF NOT EXISTS releases(
 id TEXT NOT NULL, version TEXT NOT NULL, engine TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('candidate','approved','withdrawn')),
 receipt TEXT, PRIMARY KEY(id,version,engine),
 FOREIGN KEY(id,version) REFERENCES packages(id,version));
CREATE TABLE IF NOT EXISTS active(
 channel TEXT PRIMARY KEY, id TEXT NOT NULL, version TEXT NOT NULL,
 engine TEXT NOT NULL, revision INTEGER NOT NULL,
 FOREIGN KEY(id,version,engine) REFERENCES releases(id,version,engine));
CREATE TABLE IF NOT EXISTS events(
 seq INTEGER PRIMARY KEY, occurred_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
 actor TEXT NOT NULL, operation TEXT NOT NULL, details TEXT NOT NULL);
""")

    def close(self):
        self.db.close()

    @contextmanager
    def transaction(self):
        self.db.execute("BEGIN IMMEDIATE")
        try:
            yield
            self.db.execute("COMMIT")
        except BaseException:
            self.db.execute("ROLLBACK")
            raise

    def event(self, actor, operation, details):
        self.db.execute("INSERT INTO events(actor,operation,details) VALUES(?,?,?)", (label(actor), operation, encode(details)))

    def stage(self, manifest, payload, engine_lock, actor):
        """Only stage an explicit locally supplied package; no implicit activation."""
        required = {"id", "version", "mode", "sha256", "content_sha256"}
        if not isinstance(manifest, dict) or set(manifest) != required or not isinstance(payload, bytes) or not 0 < len(payload) <= 32 * 1024 * 1024:
            raise CatalogError("invalid_package")
        identity, version = label(manifest["id"]), label(manifest["version"])
        if manifest["mode"] not in {"gpt", "claude", "chat"} or sha(payload) != manifest["sha256"]:
            raise CatalogError("package_digest_mismatch")
        # The catalog is not an engine validator. Structural/calibration approval
        # happens separately, against the exact adapter build and raw bytes.
        if not isinstance(engine_lock, dict) or engine_lock.get("schema_version") != 1:
            raise CatalogError("invalid_engine_lock")
        engine = sha(encode(engine_lock).encode())
        with self.transaction():
            previous = self.db.execute("SELECT manifest,payload FROM packages WHERE id=? AND version=?", (identity, version)).fetchone()
            if previous and previous != (encode(manifest), payload):
                raise CatalogError("immutable_version_conflict")
            self.db.execute("INSERT OR IGNORE INTO packages VALUES(?,?,?,?,?)", (identity, version, manifest["sha256"], encode(manifest), payload))
            cursor = self.db.execute("INSERT OR IGNORE INTO releases VALUES(?,?,?,'candidate',NULL)", (identity, version, engine))
            if cursor.rowcount:
                self.event(actor, "stage", {"id": identity, "version": version, "engine": engine})
        return {"id": identity, "version": version, "engine": engine, "sha256": manifest["sha256"]}

    def approve(self, reference, adapter, actor):
        """Run trusted, offline compatibility validation; callers cannot submit receipts."""
        key = self.key(reference)
        if sha(encode(adapter.lock).encode()) != reference["engine"]:
            raise CatalogError("engine_mismatch")
        row = self.db.execute("SELECT manifest,payload FROM packages WHERE id=? AND version=? AND digest=?", (key[0], key[1], reference["sha256"])).fetchone()
        if not row:
            raise CatalogError("unknown_package")
        receipt = adapter.validate_candidate(row[1], json.loads(row[0]))
        with self.transaction():
            state = self.db.execute("SELECT state FROM releases WHERE id=? AND version=? AND engine=?", key).fetchone()
            if not state or state[0] == "withdrawn":
                raise CatalogError("release_not_approvable")
            if state[0] == "approved":
                return
            self.db.execute("UPDATE releases SET state='approved',receipt=? WHERE id=? AND version=? AND engine=?", (encode(receipt), *key))
            self.event(actor, "approve", reference)

    @staticmethod
    def key(reference):
        if not isinstance(reference, dict) or set(reference) != {"id", "version", "engine", "sha256"}:
            raise CatalogError("invalid_reference")
        return tuple(label(reference[k]) for k in ("id", "version", "engine"))

    def resolve(self, reference):
        """Resolve a frozen task reference, never a moving channel name."""
        key = self.key(reference)
        row = self.db.execute("""SELECT p.manifest,p.payload,r.state FROM packages p
JOIN releases r ON r.id=p.id AND r.version=p.version
WHERE r.id=? AND r.version=? AND r.engine=? AND p.digest=?""", (*key, reference["sha256"])).fetchone()
        if not row or row[2] != "approved":
            raise CatalogError("release_not_approved")
        if sha(row[1]) != reference["sha256"]:
            raise CatalogError("package_digest_mismatch")
        return json.loads(row[0]), row[1]

    def activate(self, channel, reference, expected_revision, actor):
        """CAS prevents one administrator silently overwriting another activation."""
        label(channel)
        if type(expected_revision) is not int or expected_revision < 0:
            raise CatalogError("invalid_revision")
        key = self.key(reference)
        with self.transaction():
            self.resolve(reference)
            current = self.db.execute("SELECT revision FROM active WHERE channel=?", (channel,)).fetchone()
            if (current[0] if current else 0) != expected_revision:
                raise CatalogError("activation_conflict")
            revision = expected_revision + 1
            self.db.execute("INSERT INTO active VALUES(?,?,?,?,?) ON CONFLICT(channel) DO UPDATE SET id=excluded.id,version=excluded.version,engine=excluded.engine,revision=excluded.revision", (channel, *key, revision))
            self.event(actor, "activate", {"channel": channel, "reference": reference, "revision": revision})
        return revision

    def freeze(self, channel):
        row = self.db.execute("""SELECT a.id,a.version,a.engine,p.digest,a.revision,r.state FROM active a
JOIN packages p ON p.id=a.id AND p.version=a.version
JOIN releases r ON r.id=a.id AND r.version=a.version AND r.engine=a.engine WHERE a.channel=?""", (label(channel),)).fetchone()
        if not row or row[5] != "approved":
            raise CatalogError("channel_unavailable")
        return {"id": row[0], "version": row[1], "engine": row[2], "sha256": row[3]}, row[4]

    def withdraw(self, reference, actor, reason):
        key = self.key(reference)
        label(reason)
        with self.transaction():
            row = self.db.execute("SELECT digest FROM packages WHERE id=? AND version=?", key[:2]).fetchone()
            if not row or row[0] != reference["sha256"]:
                raise CatalogError("unknown_package")
            cursor = self.db.execute("UPDATE releases SET state='withdrawn' WHERE id=? AND version=? AND engine=? AND state<>'withdrawn'", key)
            if cursor.rowcount:
                self.event(actor, "withdraw", {"reference": reference, "reason": reason})
        # Keep the active pointer and revision as a tombstone. No automatic
        # fallback to an older package and no ABA reset of the activation CAS.
