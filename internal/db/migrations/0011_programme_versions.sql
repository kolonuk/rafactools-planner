-- Every programme is live: there is no draft or published state any more.
ALTER TABLE programmes DROP COLUMN status;

-- A copy of a programme's content after each save, reset or restore: who made
-- it, when, and what they did. A programme can be put back to any of them.
CREATE TABLE programme_versions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    programme_id INTEGER NOT NULL REFERENCES programmes(id) ON DELETE CASCADE,
    version      INTEGER NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id      INTEGER,
    user_email   TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL,
    snapshot     TEXT NOT NULL,
    UNIQUE (programme_id, version)
);
