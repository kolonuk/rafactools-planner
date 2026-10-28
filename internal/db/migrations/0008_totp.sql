-- Optional two-step sign-in (an authenticator-app code). totp_pending holds a
-- secret that has been shown but not yet confirmed with a code; totp_secret is
-- the confirmed one. totp_last_step is the newest 30-second step already used,
-- so a code cannot be replayed.
ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN totp_pending TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN totp_last_step INTEGER NOT NULL DEFAULT 0;
