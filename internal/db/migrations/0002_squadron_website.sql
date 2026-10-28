-- Optional per-squadron website/social link, shown on the dashboard and in
-- the PDF title band — see TODO.md "upload sqn badge or link to website
-- per sqn" (this is the "link" half; image upload is still TODO, needs
-- file-upload infrastructure this app doesn't have yet).
ALTER TABLE squadrons ADD COLUMN website_url TEXT NOT NULL DEFAULT '';
