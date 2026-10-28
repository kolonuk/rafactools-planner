-- Password-reset tokens. Only a SHA-256 hash of the emailed token is stored,
-- so a copy of the database cannot be used to take over an account. One
-- active token per user; rows are single use and expire after an hour.
CREATE TABLE password_resets (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_password_resets_user ON password_resets(user_id);
