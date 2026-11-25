-- UK bank holidays (copied from the government's list and kept as closures), time
-- off for instructors, each squadron's own activities, the nights the calendar feed
-- has shown, and reminders.

-- A closure is either typed in by a person ('') or put there from the bank-holiday
-- list ('bank'). Editing a 'bank' closure makes it the person's own.
ALTER TABLE programme_closures ADD COLUMN source TEXT NOT NULL DEFAULT '';

-- The cached list: one row per holiday and region, replaced whenever the
-- government's list changes.
CREATE TABLE bank_holidays (
    region TEXT NOT NULL,   -- england-and-wales | scotland | northern-ireland
    date   TEXT NOT NULL,   -- yyyy-mm-dd
    title  TEXT NOT NULL,
    PRIMARY KEY (region, date)
);
-- When the list was last fetched (a single row).
CREATE TABLE holiday_sync (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    fetched_at   DATETIME,
    attempted_at DATETIME
);
-- A bank-holiday closure a person deleted from a month: it is not put back.
CREATE TABLE programme_holiday_skips (
    programme_id INTEGER NOT NULL REFERENCES programmes(id) ON DELETE CASCADE,
    date         TEXT NOT NULL,
    PRIMARY KEY (programme_id, date)
);
-- Which list a squadron follows; 'none' turns the bank-holiday closures off.
ALTER TABLE squadrons ADD COLUMN holiday_region TEXT NOT NULL DEFAULT 'england-and-wales';

-- Time off for instructors and other staff. name is the person as they appear in
-- the Instructor boxes; user_id is set when they have a login.
CREATE TABLE instructor_absences (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    squadron_id INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    user_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
    start_date  TEXT NOT NULL,
    end_date    TEXT NOT NULL,
    note        TEXT NOT NULL DEFAULT '',
    created_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_absences_squadron ON instructor_absences(squadron_id, start_date);

-- A squadron's own activities, offered in the Activity boxes next to the built-in list.
CREATE TABLE squadron_activities (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    squadron_id INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_squadron_activities_title ON squadron_activities(squadron_id, lower(title));

-- The nights the calendar feed has shown. A night that was shown and then taken out
-- (a closure, the night unticked, the month unpublished) stays in the feed as
-- cancelled until its date has passed, so a calendar app removes it.
CREATE TABLE feed_nights (
    squadron_id INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    date        TEXT NOT NULL,
    cancelled   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (squadron_id, date)
);

-- Reminders to staff about parade nights with blank boxes: how many days ahead (0 is
-- off), and which nights have been reminded.
ALTER TABLE squadrons ADD COLUMN reminder_days INTEGER NOT NULL DEFAULT 0;
-- The expiry of the public link that its owners were last told is coming (so each expiry is told once).
ALTER TABLE squadrons ADD COLUMN share_expiry_reminded TEXT NOT NULL DEFAULT '';
CREATE TABLE reminders_sent (
    squadron_id INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    date        TEXT NOT NULL,
    sent_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (squadron_id, date)
);
