-- Moving a login to another squadron. A person with a personal email address
-- (not one of the squadron-claim addresses) can move; the owner of the squadron they
-- go to has to accept and sets their role there. While the transfer is waiting for that
-- the account is switched off (users.active = 0), and it is switched back on when the
-- transfer ends, whichever way it ends.
CREATE TABLE account_transfers (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    from_squadron_id  INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    to_squadron_id    INTEGER NOT NULL REFERENCES squadrons(id) ON DELETE CASCADE,
    requested_by      INTEGER REFERENCES users(id) ON DELETE SET NULL,
    by_owner          INTEGER NOT NULL DEFAULT 0,       -- 1: an owner asked for someone else
    role              TEXT NOT NULL DEFAULT '',         -- set by the accepting owner
    -- awaiting_person: an owner asked and the person has not agreed yet
    -- pending: waiting for an owner of the target (the account is switched off)
    -- accepted | declined | cancelled | expired
    status            TEXT NOT NULL CHECK (status IN ('awaiting_person','pending','accepted','declined','cancelled','expired')),
    person_token_hash TEXT NOT NULL,                    -- the link emailed to the person
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at        DATETIME NOT NULL,
    resolved_at       DATETIME,
    resolved_by       INTEGER REFERENCES users(id) ON DELETE SET NULL
);
-- One open transfer per person.
CREATE UNIQUE INDEX idx_account_transfers_open ON account_transfers(user_id) WHERE status IN ('awaiting_person','pending');
CREATE INDEX idx_account_transfers_to ON account_transfers(to_squadron_id, status);
CREATE INDEX idx_account_transfers_from ON account_transfers(from_squadron_id, status);
CREATE INDEX idx_account_transfers_token ON account_transfers(person_token_hash);
