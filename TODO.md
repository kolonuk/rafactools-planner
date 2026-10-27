# TODO

Sections are in rough priority order. Anything done is deleted, not ticked —
`CHANGELOG.md` and git history are the record.

## Before anyone relies on it

- [ ] **Password reset.** Emailed single-use token, same shape as email
  verification (`email_verifications` is the pattern to copy). Invalidate the
  user's other sessions when the password changes.
- [ ] **Real SMTP.** With `SMTP_HOST` unset the app only logs links. Pick a
  relay for test/prod and set `SMTP_HOST/USER/PASS` and `MAIL_FROM`.
- [ ] **Set the prod `SESSION_KEY` secret** (`RAFACTOOLS_SESSION_KEY` in
  dadock). Needs a human: `gh secret set RAFACTOOLS_SESSION_KEY --repo kolonuk/dadock --body "$(openssl rand -hex 32)"`.
  The app now refuses to start with a key under 32 characters.
- [ ] **Branch protection.** Require the `CI` jobs to pass before merging to
  `test` and `main` (GitHub repo setting).
- [ ] **Privacy basics.** The app stores cadet names, so add a short privacy
  notice, a way to delete a squadron's data, and a retention rule for old
  programmes.

## Environments and hosting

- [ ] **Test environment:** disable the claim form there (a
  `DISABLE_SIGNUPS` flag) and copy prod's database into it on each deploy, so
  test always has realistic data. The copy step lives in dadock's deploy
  workflow.
- [ ] **Domain and TLS.** Everything is LAN-only over plain HTTP today. Once
  there is a domain: put it behind TLS, set `BASE_URL` to the `https://`
  address and `COOKIE_SECURE=true` in dadock's compose.

## Bugs and rough edges

- [ ] **Day label should come from the date**, not a free-text field. A label
  that disagrees with the real weekday has already broken week grouping once.
  Derive it with `defaultDayLabels` and drop the editable input.
- [ ] **Non-Latin-1 text breaks the PDF.** Smart quotes, em-dashes and
  accented letters come out as mojibake. Fix with an embedded Unicode font
  (`AddUTF8Font`).
- [ ] **Second half of a parade night:** copy half 1's text when adding it,
  and add a "remove second half" button that warns if half 2 differs.
- [ ] **PDF:** multi-week closures and weekend events aren't true merged
  cells, and long words break mid-word in the Weekends column. Compare
  against `examples/pdfs/` after any `internal/pdfgen` change (see the
  `pdf-fidelity` skill).
- [ ] **Mobile and accessibility pass:** the grid editor is a wide table;
  icon-only delete buttons need labels; check focus states and swatch
  contrast.
- [ ] **Squadron crest/logo** in the PDF header. Needs a first upload path
  (the app has none yet).

## Planned improvements to existing screens

- [ ] **Copy last month forward:** requirement categories (not cadet names)
  and standing notes. Confirm with a real user first.
- [ ] **Reorder rows with up/down buttons.** Simple and needs no JavaScript.
  Real drag-and-drop would mean loosening the `script-src 'none'` CSP, so
  only do that together with the picker below.
- [ ] **Clearer classification grouping:** an explicit "these classes do the
  same thing" control instead of typing identical text to merge cells.

## Feature ideas

Roughly ordered by value for the effort.

1. **Instructors list.** A per-squadron list (name, rank, optional email)
   plus "All Staff". Start with a native `<datalist>` on the Instructor row,
   which needs no JavaScript. Also lets us set `display_name` on users.
2. **Activities and training classes library.** A per-squadron catalogue
   (title, syllabus/class, default instructor, notes) to pick from when
   filling the grid. A proper searchable popup needs a small first-party
   script, so decide on the CSP change once for this and for drag-and-drop.
3. **Ideas board** (reminder notes for future months). Each idea has a title,
   type, possible dates, contact details (e.g. RAF Careers), equipment needed,
   cost, status (idea / asked / confirmed / dropped) and a "use in programme"
   button that drops it into a day.
4. **QR-code suggestions from cadets.** Each squadron gets a revocable public
   link and QR code to a short form: type (camp, training day, airshow or
   airbase visit, trip, other), proposed dates, cost, transport, location,
   contact name, description. Submissions land in an inbox on the Ideas board
   for staff to accept or reject. It's a public write endpoint, so it needs:
   rate limiting (already have `ipLimiter`), a hidden honeypot field, strict
   length limits, no cadet names or phone numbers collected, and moderation
   before anything is shown.
5. **Microsoft sign-in** (see the note below), alongside the password login.
6. **Read-only sharing:** a published/final status plus a read-only link for
   parents and cadets, and an `.ics` calendar feed.
   **QR codes at the bottom of the programme PDF** (and the web page): one
   for subscribing to the `.ics` feed, one for the cadet suggestion form
   (item 4). Notes:
   - The PDF gets shared around, so the feed URL must carry a long random,
     revocable per-squadron token, and expose only parade dates and
     activity titles, never cadet names or the requirements tracker.
   - Generate the QR in Go (e.g. `skip2/go-qrcode`) and embed it as an image
     in `pdfgen`. Keep the codes big enough to scan from a printed A4 and
     leave room for them in the single-page layout.
   - A "regenerate link" button in squadron settings, since a leaked link
     can't otherwise be withdrawn.
7. **Reminders:** email staff N days before a parade night that still has
   blank activity or instructor cells.
8. **Programme templates** for recurring evenings (e.g. "Monday: drill, then
   first aid").
9. **Instructor availability and duty rota,** with a clash warning when the
   same person is on two things at once.
10. **Audit trail:** "last edited by X at Y" on each programme, and an
    optional change log.
11. **Account hygiene:** change password, sign out everywhere, optional
    two-factor.
12. **Data export and backup:** download a squadron's data as JSON/CSV.

### Can Microsoft sign-in work without being a RAFAC tenant admin?

Probably yes for registering the app, but not guaranteed for letting RAFAC
users in. This is from general Entra ID behaviour, not checked against the
RAFAC tenant:

- You can register the app in **your own** Entra tenant (a free tenant is
  enough) and mark it **multi-tenant**. You don't need admin rights in the
  RAFAC tenant to do that.
- A RAFAC user signing in is then asked to consent. If the RAFAC tenant
  blocks user consent (common in government tenants), a tenant admin has to
  approve the app once, or the user sees an "approval required" screen. That
  is the part you can't control. Ask the RAFAC/MOD IT contact; the app only
  needs the basic `openid profile email` permissions.
- The "Microsoft SSO already in use" may be a separate app you can't reuse.
  Reusing it only works if its owner adds this site as a redirect URI.
- Don't trust the `email` claim alone. Match on the tenant ID (`tid`) and the
  stable user ID (`oid`), and only allow a claim when the tenant is RAFAC's.
  That keeps the current rule (only `oc.<n>@`/`snco.<n>@` can claim a
  squadron) intact.
- If admin approval is refused, keep the password login. It already works
  for everyone.

## Reference

`examples/pdfs/` holds synthetic PDFs from `cmd/genexamples` (fabricated
data). Never commit a real squadron programme; real cadet and staff names were
scrubbed from this repo's history once already.
