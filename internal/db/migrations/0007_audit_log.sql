-- Who did something that matters to a squadron's security: the master admin
-- looking at it, and owners changing who can get in. Shown to the squadron's
-- owners and to the master admin; entries older than a year are pruned.
CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    squadron_id INTEGER REFERENCES squadrons(id) ON DELETE CASCADE,
    actor_id    INTEGER,
    actor_email TEXT NOT NULL,
    action      TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_audit_squadron ON audit_log(squadron_id, id);
CREATE INDEX idx_audit_at ON audit_log(at);
