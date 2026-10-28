-- Site settings the master admin can change from the website. A value here
-- overrides the matching environment variable; a missing or empty one falls
-- back to the environment. Secrets (is_secret = 1) are stored encrypted.
CREATE TABLE site_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    is_secret  INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by TEXT NOT NULL DEFAULT ''
);
