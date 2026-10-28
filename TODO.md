# TODO

Sections are in rough priority order. Anything done is deleted, not ticked —
`CHANGELOG.md` and git history are the record.

## Left over from the move-activities work

Built, tested and documented: moving a night's activities to another date
(store, endpoint, no-script "Move" control, TypeScript drag-and-drop; checked in
Chromium), plus the "Add a second half" fix. Still to do:

- [ ] **Watch the first CI run on GitHub.** The new jobs (staticcheck, gosec, tsc,
  docs check, HTML validation) have only been run locally so far.
- [ ] **Automated browser test** for the drag-and-drop script (it was verified by
  hand in Chromium only). Worth adding if the script grows.
- [ ] **First docs snapshot** (`go run ./cmd/devtools docs-snapshot 1.<PR>`) once a
  release is designated, so the version selector has something to compare.

## Before anyone relies on it

- [ ] **Real SMTP.** With `SMTP_HOST` unset the app only logs links. Pick a
  relay for test/prod and set `SMTP_HOST/USER/PASS` and `MAIL_FROM`.
- [ ] **Set the prod `SESSION_KEY` secret** (`RAFACTOOLS_SESSION_KEY` in
  dadock). Needs a human: `gh secret set RAFACTOOLS_SESSION_KEY --repo kolonuk/dadock --body "$(openssl rand -hex 32)"`.
  The app now refuses to start with a key under 32 characters.
- [ ] **Branch protection.** Require the `CI` jobs to pass before merging to
  `test` and `main`. Blocked: the API refuses it (403) because it needs GitHub
  Pro or a public repo, the same limit that stops CodeQL results uploading.
  The repo must stay private (see the PDF anonymisation history), so this needs
  a plan upgrade.
- [ ] **Privacy basics.** The app stores cadet names, so add a short privacy
  notice, a way to delete a squadron's data, and a retention rule for old
  programmes.

## Environments and hosting

- [ ] **Later: version check.** Two parts, to be done together, not yet:
  1. A repo skill (`.claude/skills/version-check`) saying when and how the
     version must change (what counts as a user-visible change, how the
     `1.<PR number>` scheme and the docs `since:`/`changed:` notes relate), so it is
     checked before a PR is opened.
  2. A GitHub Actions workflow, run on pull requests, that reads the version
     the PR submits and compares it with the version on `main`, failing when it
     is unchanged or goes backwards where a change is expected (and passing for
     docs-only or CI-only PRs as the skill defines). Written in Go like the
     other tooling (a `devtools` subcommand), run by the workflow, with tests.
     Decide first, and ask the owner: where the version lives in the repo
     today, since it is currently computed at build time from the PR number.
- [ ] **Test environment:** disable the claim form there (a
  `DISABLE_SIGNUPS` flag) and copy prod's database into it on each deploy, so
  test always has realistic data. The copy step lives in dadock's deploy
  workflow.
- [ ] **Domain and TLS.** Everything is LAN-only over plain HTTP today. Once
  there is a domain: put it behind TLS, set `BASE_URL` to the `https://`
  address and `COOKIE_SECURE=true` in dadock's compose.

## Bugs and rough edges

- [ ] **PDF: scripts outside Western European text** (Greek, Cyrillic, Chinese)
  print as `?`. Needs an embedded Unicode font (`AddUTF8Font`); smart quotes,
  dashes and accents already work.
- [ ] **Cancel a parade night entirely** once it has rows. Unticking Parade
  leaves the empty rows in the database (harmless, untidy).
- [ ] **PDF:** multi-week closures and weekend events aren't true merged
  cells, and long words break mid-word in the Weekends column. Compare
  against `examples/pdfs/` after any `internal/pdfgen` change (see the
  `pdf-fidelity` skill).
- [ ] **Mobile layout.** Check uniform-swatch contrast. The grid scrolls sideways
  on a phone; a real mobile layout is still open. (Screen-reader and keyboard
  walk-throughs are not wanted for now; focus on usable features and bugs.)
- [ ] **Squadron crest/logo image** in the PDF header. The website link is done;
  an image needs a first upload path (storage plus an upload form), which the
  app does not have yet.

## Planned improvements to existing screens

- [ ] **Copy last month forward:** requirement categories (not cadet names)
  and standing notes. Confirm with a real user first.
- [ ] **Reorder rows within a night / requirement entries:** moving a night's
  activities to another date is done (drag or the Move control). Reordering
  requirement categories and entries is not; reuse the same pattern.
- [ ] **Clearer classification grouping:** an explicit "these classes do the
  same thing" control instead of typing identical text to merge cells.

## Feature ideas

Roughly ordered by value for the effort.

1. **Instructors list, next step.** The list (name and rank) and display names
   exist. Still to do: rename an entry (today you remove and re-add), and
   per-instructor availability (see the duty rota idea below).
2. **Activities and training classes library.** A per-squadron catalogue
   (title, syllabus/class, default instructor, notes) to pick from when
   filling the grid. A proper searchable popup needs a small first-party
   script, which is now allowed (see the README's "Scripts" note); keep the
   form usable without it where reasonable.
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
5. **Read-only sharing:** a published/final status plus a read-only link for
   parents and cadets, and an `.ics` calendar feed.
   **QR codes at the bottom of the programme PDF** (and the web page): one
   for subscribing to the `.ics` feed, one for the cadet suggestion form
   (item 4). Notes:
   - The PDF gets shared around, so the feed URL carries a long random
     per-squadron token and exposes only parade dates and activity titles,
     never cadet names or the requirements tracker.
   - The link expires after 3 whole months. Users regenerate it themselves,
     as and when needed (a button in squadron settings), and each
     regeneration gives a new token, a new QR code and a fresh 3 months; the
     old link stops working straight away. There is no separate revoke. The
     expiry date is printed at the bottom of the PDF next to the QR code, so
     people know when to re-scan. An expired link should say so clearly
     rather than return an empty calendar. A reminder email to the owner a
     week before expiry would help.
   - Generate the QR in Go (e.g. `skip2/go-qrcode`) and embed it as an image
     in `pdfgen`. Keep the codes big enough to scan from a printed A4 and
     leave room for them in the single-page layout.
6. **Reminders:** email staff N days before a parade night that still has
   blank activity or instructor cells.
7. **Programme templates** for recurring evenings (e.g. "Monday: drill, then
   first aid").
8. **Instructor availability and duty rota,** with a clash warning when the
   same person is on two things at once.
9. **Audit trail:** "last edited by X at Y" on each programme, and an
    optional change log.
10. **Account hygiene:** two-factor sign-in; sign out everywhere. (Change
    password is done; see "Your details".)
11. **Data export and backup:** download a squadron's data as JSON/CSV.
12. **Documentation, next steps.** The versioned in-app docs exist
    (`/docs`, see `.claude/skills/docs/SKILL.md`). Still to do: add pages for
    each feature on this list as it is built; take the first release snapshot
    with `go run ./cmd/devtools docs-snapshot 1.<PR number>` once a release is
    designated, so the version selector and compare view have something to
    compare; link relevant screens to their help page; images/screenshots
    (not supported yet); in-docs search (a first-party script is now allowed).

### Missing features spotted along the way

Small gaps found while building and reviewing; each is a candidate to pick up.

- A **status** that means something (draft / final). The database has it, but
  it is always "draft". Months are deliberately never deleted: keep them all.
- **Edit uniform colours** (names and colours) in squadron settings; today only
  the five defaults exist.
- **Resend an invite** (and see when it was sent); today you revoke and re-invite.
- **Delete a squadron's data** (owner, with confirmation), the part of "Privacy
  basics" that needs code.
- **Edit a closure or weekend note** instead of deleting and re-adding it.
- **Sign-in session list** ("where am I signed in?") with per-session sign-out.
- **Print-friendly programme page** (the PDF covers most of this).
- **Search** across programmes for an activity or a cadet's name.
- **Last-seen / last-edited** on the dashboard, to spot stale programmes.

## Reference

`examples/pdfs/` holds synthetic PDFs from `cmd/genexamples` (fabricated
data). Never commit a real squadron programme; real cadet and staff names were
scrubbed from this repo's history once already.

## Master admin: still to do

The account, the squadron switcher (read-only), the banner and the audit log
lines are built and tested. What is left, and why it is not done here:

- [ ] **Wire the settings into the deployment (needs a human, in `dadock`).** The
  running containers get their environment from `dadock`'s compose file, which
  is a different repository this session cannot change, and the secret has to
  be created by someone with admin rights on it. Steps:
  1. In `dadock`'s GitHub settings add a **secret** `RAFACTOOLS_MASTER_ADMIN_PASSWORD`
     and a **variable** `RAFACTOOLS_MASTER_ADMIN_EMAIL` (`john@kolon.co.uk`). This session
     cannot create them (GitHub's variables/secrets API is blocked from here, and
     `dadock` is outside its scope), so a helper does it from your machine:
     `go run ./cmd/devtools github-setup -repo kolonuk/dadock -apply` creates the
     variable with a placeholder (edit it to the real email) and prompts you
     privately for the secret.
  2. In `dadock`'s compose file give each `rafactools*` service
     `MASTER_ADMIN_EMAIL` and `MASTER_ADMIN_PASSWORD` from those two.
  3. Redeploy. The app applies them on every start, so every deployment creates
     or updates the account (the log says what happened; check it).

  The password must be a **secret, not a variable**: GitHub variables are stored
  and shown in plain text. The password discussed in chat was 10 characters, under
  the app's 12-character minimum, so the app refuses it (and logs why); pick a
  longer one and treat the one that was pasted in chat as exposed.
- [x] **Issue reporting** (done: "Report a problem", emailed to `ISSUE_EMAIL`, not stored).
- [ ] **Report diagnostics, next round**: a request ID shown in the page, the
  response header, the log and the report so all three can be matched; DB
  migration version and "SMTP configured" flags in the server section; an
  anonymised shape of the programme being viewed (counts of nights, halves,
  classifications; no text); failed fetch/network statuses and page timing;
  a "can you repeat it?" and "how bad is it?" question; recent PDF-render
  errors. Then, if wanted, store reports so the master admin has an inbox with
  a status (new / looking / done) instead of only email.
- [ ] **Two-factor sign-in for the master admin** before it is used on a real
  deployment: it can read every squadron, so a stolen password is the worst case.
- [ ] **Let the master admin act as an editor or owner** in a squadron when
  helping fix something (today it is strictly read-only, on purpose). If added,
  make it an explicit second step with its own audit line.
- [ ] **Show the audit trail** (who looked at which squadron, when) to the master
  admin, and to a squadron's owners, instead of only writing it to the log.
- [ ] **Shorter sessions for the master admin** (the normal 30 days is long for
  this account).
