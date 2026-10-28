-- What the person signed in with, so the account page can show "where am I
-- signed in?". A short, coarse label is shown; the raw header is capped.
ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
