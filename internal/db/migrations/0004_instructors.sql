-- A squadron's list of instructors/staff for the programme grid's Instructor
-- row suggestions. Separate from user accounts so staff who have no login
-- can appear too. Deliberately just a name and a rank: no contact details,
-- to keep personal data to the minimum the feature needs.
CREATE TABLE instructors (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    squadron_id INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    rank        TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (squadron_id, name, rank)
);
CREATE INDEX idx_instructors_squadron ON instructors(squadron_id);
