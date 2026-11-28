-- When an invitation was last emailed (so it can be sent again, and the owner can see when).
ALTER TABLE invites ADD COLUMN sent_at DATETIME;
UPDATE invites SET sent_at = created_at;

-- When a signed-in session was last used.
ALTER TABLE sessions ADD COLUMN last_used_at DATETIME;

-- Activity suggestions from cadets: a public form per squadron, at a link with its own
-- token (made and revoked by an owner), and an inbox for editors. Only the name of an
-- activity is collected.
ALTER TABLE squadrons ADD COLUMN suggest_token TEXT NOT NULL DEFAULT '';
ALTER TABLE squadrons ADD COLUMN suggest_qr_on_pdf INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX idx_squadrons_suggest_token ON squadrons(suggest_token) WHERE suggest_token <> '';

CREATE TABLE activity_suggestions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    squadron_id    INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Set once an editor has made it a real activity: the squadron's own activity it became
    -- (null if that was later removed or it matched a built-in one), and its title then.
    activity_id    INTEGER REFERENCES squadron_activities(id) ON DELETE SET NULL,
    activity_title TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_activity_suggestions_squadron ON activity_suggestions(squadron_id, created_at);
