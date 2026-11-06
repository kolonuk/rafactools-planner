-- Public sharing: parade times, a per-squadron public link, and a "published"
-- flag on each month. Only published months appear at the public link.
ALTER TABLE squadrons ADD COLUMN parade_start TEXT NOT NULL DEFAULT '19:00'; -- HH:MM, the squadron's local time
ALTER TABLE squadrons ADD COLUMN parade_end   TEXT NOT NULL DEFAULT '21:30';
ALTER TABLE squadrons ADD COLUMN share_token  TEXT NOT NULL DEFAULT '';      -- empty: no public link yet
ALTER TABLE squadrons ADD COLUMN share_expires_at DATETIME;                  -- the link stops working after this
CREATE UNIQUE INDEX idx_squadrons_share_token ON squadrons(share_token) WHERE share_token <> '';

ALTER TABLE programmes ADD COLUMN published INTEGER NOT NULL DEFAULT 0;
