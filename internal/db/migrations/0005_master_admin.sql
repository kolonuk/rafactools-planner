-- A site-wide "master admin" who can look at any squadron (read-only) to see
-- problems reported there. The account is created and kept in step with the
-- deployment's configuration at startup (see auth.EnsureMasterAdmin): it is
-- never created through the web UI, an invite or a claim.
ALTER TABLE users ADD COLUMN is_master_admin INTEGER NOT NULL DEFAULT 0;

-- At most one master admin can exist, enforced by the database.
CREATE UNIQUE INDEX idx_one_master_admin ON users(is_master_admin) WHERE is_master_admin = 1;

-- The squadron a master admin is currently looking at, per sign-in session.
-- NULL means their own (home) squadron. Cleared if that squadron is deleted.
ALTER TABLE sessions ADD COLUMN acting_squadron_id INTEGER REFERENCES squadrons(id) ON DELETE SET NULL;
