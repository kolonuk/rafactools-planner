-- Core schema. SQLite, applied in order by internal/db migrator.
--
-- Multi-tenant: a squadron is "claimed" by the first person who signs up
-- with an email matching snco.<number>@ or oc.<number>@ at the configured
-- RAFAC domain (see internal/auth). That claim creates the squadron row and
-- makes the claimant its owner. Everyone else joins via an invite sent by an
-- existing member of that squadron — invites accept any email address, since
-- the inviting owner is vouching for them.

CREATE TABLE squadrons (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    number             TEXT NOT NULL UNIQUE, -- e.g. "2491" — the claimed unit number, one claim each
    name               TEXT NOT NULL,        -- e.g. "2491 (Lyneham) Squadron"
    subtitle           TEXT NOT NULL DEFAULT 'Royal Air Force Air Cadets',
    protective_marking TEXT NOT NULL DEFAULT '', -- e.g. "OFFICIAL - Public" header/footer banner; blank disables it
    parade_weekdays    TEXT NOT NULL DEFAULT '[1,4]', -- JSON array, ISO weekday ints 1=Mon..7=Sun
    classifications    TEXT NOT NULL DEFAULT '["Junior Cadets","1st Class Cadets","Leading Cadets","Senior Cadets"]', -- JSON array, left-to-right, progression order
    uniform_colours    TEXT NOT NULL DEFAULT '[
        {"key":"green","label":"No 3 (MTP) - Greens","hex":"#8faa6e"},
        {"key":"dark_blue","label":"No 2 (Working Blues)","hex":"#3b5e82"},
        {"key":"light_blue","label":"No 2A - Wedgewoods","hex":"#a9c2db"},
        {"key":"white","label":"Sports / Civvies","hex":"#ffffff"},
        {"key":"grey","label":"Closure","hex":"#c9c9c9"}
    ]',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE users (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    squadron_id       INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    email             TEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    display_name      TEXT NOT NULL DEFAULT '',
    role              TEXT NOT NULL DEFAULT 'editor' CHECK (role IN ('owner','editor','viewer')),
    email_verified_at DATETIME, -- set immediately for invited users; via /verify for the claiming owner
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE email_verifications (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Pending invitations, sent by an existing squadron member to any email
-- address. Accepting one creates the user directly (already verified — the
-- invite link itself is the proof of inbox access) and consumes the row.
CREATE TABLE invites (
    token        TEXT PRIMARY KEY,
    squadron_id  INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'editor' CHECK (role IN ('owner','editor','viewer')),
    invited_by   INTEGER NOT NULL REFERENCES users(id),
    expires_at   DATETIME NOT NULL,
    accepted_at  DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_invites_squadron ON invites(squadron_id);

CREATE TABLE sessions (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE programmes (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    squadron_id  INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    year         INTEGER NOT NULL,
    month        INTEGER NOT NULL CHECK (month BETWEEN 1 AND 12),
    status       TEXT NOT NULL DEFAULT 'draft', -- draft | published
    created_by   INTEGER REFERENCES users(id),
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (squadron_id, year, month)
);

-- One row per calendar date in the programme's month.
CREATE TABLE programme_days (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    programme_id  INTEGER NOT NULL REFERENCES programmes(id) ON DELETE CASCADE,
    date          TEXT NOT NULL, -- ISO yyyy-mm-dd
    is_parade     INTEGER NOT NULL DEFAULT 0,
    day_label     TEXT NOT NULL DEFAULT '', -- e.g. "M", "T"
    uniform_key   TEXT NOT NULL DEFAULT '', -- matches squadrons.uniform_colours[].key
    halves        INTEGER NOT NULL DEFAULT 1 CHECK (halves IN (1,2)),
    notes         TEXT NOT NULL DEFAULT '',
    UNIQUE (programme_id, date)
);

-- Activity/Instructor text rows within a parade day's half. `texts` is a
-- JSON array of strings, one per entry in squadrons.classifications (left to
-- right) — the classification count is configurable (4 in the original
-- template, 5 once "Master" was added), so this isn't a fixed-width row.
-- Adjacent columns holding equal text are merged into one wide cell at
-- render time, which is how the source PDFs show a single shared activity
-- across some or all classifications without a separate "merged" flag.
CREATE TABLE programme_rows (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    day_id        INTEGER NOT NULL REFERENCES programme_days(id) ON DELETE CASCADE,
    half          INTEGER NOT NULL CHECK (half IN (1,2)),
    row_type      TEXT NOT NULL CHECK (row_type IN ('activity','instructor')),
    texts         TEXT NOT NULL DEFAULT '[]',
    UNIQUE (day_id, half, row_type)
);

-- Multi-week closure blocks (holidays, stand-downs) that override normal
-- day rendering with one merged banner cell.
CREATE TABLE programme_closures (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    programme_id INTEGER NOT NULL REFERENCES programmes(id) ON DELETE CASCADE,
    start_date   TEXT NOT NULL,
    end_date     TEXT NOT NULL,
    message      TEXT NOT NULL DEFAULT '' -- newline-separated lines, rendered centered
);

-- Free-text lines shown against a specific Saturday or Sunday.
CREATE TABLE programme_weekend_notes (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    programme_id INTEGER NOT NULL REFERENCES programmes(id) ON DELETE CASCADE,
    date         TEXT NOT NULL, -- the Saturday or Sunday this note is shown against
    text         TEXT NOT NULL DEFAULT '',
    sort_order   INTEGER NOT NULL DEFAULT 0
);

-- Footer "NOTES" requirement tracker: named categories (e.g. "Mandatory
-- Briefings (TBC - 12/06/2025)") each listing cadets still to complete it.
CREATE TABLE programme_requirement_categories (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    programme_id INTEGER NOT NULL REFERENCES programmes(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    due_label    TEXT NOT NULL DEFAULT '',
    sort_order   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE programme_requirement_entries (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    category_id INTEGER NOT NULL REFERENCES programme_requirement_categories(id) ON DELETE CASCADE,
    cadet_name  TEXT NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_programmes_squadron ON programmes(squadron_id);
CREATE INDEX idx_programme_days_programme ON programme_days(programme_id);
CREATE INDEX idx_programme_rows_day ON programme_rows(day_id);
CREATE INDEX idx_weekend_notes_programme ON programme_weekend_notes(programme_id, date);
CREATE INDEX idx_req_categories_programme ON programme_requirement_categories(programme_id);
