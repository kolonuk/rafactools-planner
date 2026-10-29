-- A member an owner has marked inactive cannot sign in or reset their password
-- until reactivated. An owner can also send a member a temporary password:
-- must_change_password makes them choose their own at their next sign-in, and
-- the temporary one stops working after temp_password_expires_at.
ALTER TABLE users ADD COLUMN active INTEGER NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN temp_password_expires_at DATETIME;

-- A pending change of email address, confirmed by a link sent to the new one.
CREATE TABLE email_changes (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    new_email  TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_email_changes_user ON email_changes(user_id);
