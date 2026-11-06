# TODO

What is planned for the RAFACTools Planner. Ideas and pull requests are welcome:
open an issue on GitHub first if you want to talk an item through. Anything done
is deleted, not ticked; `CHANGELOG.md` and git history are the record.

## Priorities

Every item carries a priority tag. Work top-down through the tags, not the sections:

- **P1 security**: protects data, accounts and the deployment. Do first.
- **P2 helpful feature**: new capability that saves people real work.
- **P3 usability**: makes existing screens easier or fixes rough edges.
- **P4 nicety**: nice to have; only when nothing above is waiting.

Find them with `grep -n "P1 security" TODO.md` (and so on). Items marked "needs a
human" or "later" are blocked on someone else or on a decision, whatever their tag.

## Next up (to be triaged)

- [ ] **[P1 security]** **Turn on CodeQL result uploads.** The repository
  is public now, so GitHub's code scanning is available. Let the CodeQL job upload its results
  (see the comment in `.github/workflows/ci.yml`, which still describes the old private-repo
  limit), and update that comment.
- [ ] **[P4 nicety]** **Public-repo tidy-up.** Re-read the in-app help pages (`run-your-own`, `about-these-docs`)
  and the `.claude/skills` notes for wording that assumes a private project or a particular
  host, and make them neutral. Edit the `next` docs; released snapshots stay as published.

- [ ] **[P2 helpful feature]** **Planner redesign and public sharing: the plan, in order.** Decided: no Bootstrap;
  keep the hand-written Material-style CSS. One phase per pull request through dev, test and main.
  1. *Foundations (done in part):* theme toggle with the system setting first, breadcrumb, and the
     Actions menu with the parade switch as a tick or cross. Still to do: phone layout of the top bar.
  2. *The grid:* replace the table with a flowing list where each day is its own row/card (date and day
     badge, uniform chip, then the activity and instructor per class), laid out with CSS grid and
     container queries so one layout serves desktop and phone. Drag the day by its handle. Fix the
     "Day" column after a move. Inline edit (tick/cross) arrives with the same component.
  3. *Sharing settings:* squadron settings gain parade start and end time (default 19:00 to 21:30, shown
     in the squadron's time zone, Europe/London) beside the parade days; each month gets a Published
     setting (off by default; the old `status` column is unused); a per-squadron public link token that
     expires after 1 month and can be regenerated.
  4. *Public files:* the PDF and a PNG of each published month at the public link (same token, same
     1-month expiry, nothing for unpublished months). The PDF is cached, see the next item. The PNG
     is drawn from the PDF.
  5. *Calendar feed (`.ics`):* one event per parade day, from the start to the end time, from today
     onwards, published months only. The description gives the uniform and the activities grouped
     per class (no instructor names unless a setting allows them), plus the link to the month's
     public PDF. Stable event ids and a sequence number so calendar apps update, not duplicate; a
     cancelled night is marked cancelled. An expired link says so instead of returning nothing.
  6. *QR code* for the feed at the foot of the PDF (and the web page) with its expiry date printed.
- [ ] **[P2 helpful feature]** **Cache the PDF (and PNG) until the month changes.** Keep the generated file per
  month with a content version; any save, move, restore or reset invalidates it. Regenerate lazily
  on the next request, and for a published month regenerate straight after the save so the public
  link is always warm. A change to the squadron's settings, the uniform colours, or the app
  version also invalidates it.
- [ ] **[P2 helpful feature]** **"Copy" a previous month into this one.** Put a **Copy** button at the top of
  the monthly planner page. It opens a popup listing the squadron's earlier months; picking one
  copies it into the month being viewed.
  - Copy only the uniform, the activities and the instructors. Leave everything else (notes,
    weekend events, closures, the requirement tracker) alone.
  - Line the months up by parade night: the first parade night of the source goes on the first
    parade night of this month, the second on the second, and so on, so every date and weekday
    comes out right for the current month.
  - If the source has more parade nights than this month, drop the extra ones from the end.
  - Ask for confirmation first if this month already has entries, and say what will be replaced.
  - Test: a 5-night source into a 4-night month, a 4-night source into a 5-night month, and
    different weekday patterns.
- [ ] **[P3 usability]** **More detail in the programme history.** The history page should say what
  actually changed, not just that a version was saved. For example: "Moved 3 Nov activities to
  10 Nov", "Added closure 22-26 Dec", "Changed uniform for 17 Nov from Blue to Working dress",
  "Edited instructor on 10 Nov (First Class)". Work out the changes by comparing the saved
  version with the one before it, and use the same wording on the compare view when that exists
  (see "Compare two programme versions").
- [ ] **[P3 usability]** **Menu and breadcrumbs on the monthly planner page.** Replace the row of action links at the
  top with a proper menu (for example a dropdown or toolbar menu with the actions grouped:
  Copy, Export PDF, History, and so on). Include the slider (the parade-nights-only toggle) as a
  menu option shown with a tick or a cross. Add a breadcrumb trail above it (for example
  Squadron > Programmes > November 2026) so it is clear where you are and easy to go back.
  The menu must work with the keyboard and on a phone, and still reach every action when
  scripts are off if that is reasonable. Do this together with "Modernise the planner grid".
- [ ] **[P3 usability]** **Edit in place for every text entry, not only delete.** Closures, weekend notes,
  programme notes and the requirement tracker (categories and entries) can be deleted today but
  not edited. Clicking the text should turn it into an editable field with a tick (save) and a
  cross (cancel); Enter saves and Escape cancels. Do it the same way everywhere in the grids and
  lists, as part of making the site friendlier (see "Modernise the planner grid" and the menu
  item). Keep a plain "Edit" link or button that works without scripts and with the keyboard, and
  record each edit in the programme history. This replaces the old "Edit a closure or weekend
  note" idea.
- [ ] **[P3 usability]** **PDF: the colour key overlaps the planner.** Fix the layout in
  `internal/pdfgen` so the key never sits on top of the grid; compare against `examples/pdfs/`
  (see the `pdf-fidelity` skill).
- [ ] **[P3 usability]** **Bug: moving a day in the grid does not update the "Day" column.** After a
  drag or the Move control, the weekday label keeps the old value until reload.
- [ ] **[P3 usability]** **Modernise the planner grid.** It feels dated. Idea: each day is its own
  "line" (row/card) and its contents are what is dragged to rearrange. Look at a more modern
  table pattern generally (sortable, keyboard-accessible, works on a phone, which also helps
  the "Phone layout" item). Decide the design before building.
- [ ] **[P3 usability]** **Lift the JavaScript restriction and review reactive features.** The
  README still says "no JS framework" and "pages should still work without script where
  reasonable". Remove that restriction (keep the CSP `script-src 'self'`, no inline script, no
  third-party or `eval`), update the README, `docs` skill and lint skill wording, then review
  which screens would be better with TypeScript (live validation, inline edit, drag and drop,
  partial refresh) and add the worthwhile ones as items.
- [ ] **[P3 usability]** **Light/dark mode toggle.** Add a toggle (stored per user in the browser,
  e.g. `localStorage` or a cookie), but the first appearance must stay the system/browser
  preference. Offer system / light / dark, with "system" the default. The theme is
  `internal/web/static/css/app.css`; a script is allowed for this.
- [ ] **[P2 helpful feature]** **Activity list (default plus per-squadron).**
  - Ship a JSON file of default activities, embedded in the binary. Starting set:
    First Class - Flying Operation; First Class - Map Reading; First Class - Initial Expedition
    Training; First Class - Radios; Leading - Pilot Navigation; Leading - Principles of Flight;
    First Class - History of the RAF; First Class - History of ATC; Annual Briefings;
    Climatic Injuries; DofE Prep; Laser Quest; Bowling; Section Visit; Guest Speaker - RAF Police.
  - Custom activities a squadron creates are stored in the database, unique to that squadron.
  - Clean-up: if a squadron's custom activity matches a default (compare case-insensitively,
    ignoring surrounding whitespace), delete the database entry, for all squadrons. Run it
    at start-up or as a migration, and log what was removed.
  - Pick from the list when filling the grid. This replaces the earlier "Activities and training
    classes library" idea under Feature ideas; merge the two.

## The app and its image

This repository builds the app and its container image and stops there. Hosting,
TLS, secrets and redeploying belong to whoever runs the image (the docs page
"Run your own copy" says what the image needs), so they are not tracked here.

- [ ] **[P4 nicety]** **Later: version check.** Two parts, to be done together, not yet:
  1. A repo skill (`.claude/skills/version-check`) saying when and how the
     version must change (what counts as a user-visible change, how the
     `1.<N>` release scheme and the docs `since:`/`changed:` notes relate), so it is
     checked before a PR is opened.
  2. A GitHub Actions workflow, run on pull requests, that reads the version
     the PR submits and compares it with the version on `main`, failing when it
     is unchanged or goes backwards where a change is expected (and passing for
     docs-only or CI-only PRs as the skill defines). Written in Go like the
     other tooling (a `devtools` subcommand), run by the workflow, with tests.
     Decide first, and ask the owner: where the version lives in the repo
     today: it is now taken at build time from the newest docs snapshot.

## Bugs and rough edges

- [ ] **[P1 security]** **Turnstile on the invite-accept page.** Accepting an invite creates an
  account, so it is the one account-creating form without the human check that login, claim and
  delete-squadron now have (`turnstileOK` in `internal/web/turnstile.go`). Add the
  `{{template "turnstile" .}}` to the form, call `turnstileOK` after the CSRF check in the accept
  handler, add it to `TestTurnstileGuardsLoginClaimAndDelete`, and update the docs.
- [ ] **[P3 usability]** **PDF: scripts outside Western European text** (Greek, Cyrillic, Chinese)
  print as `?`. Needs an embedded Unicode font (`AddUTF8Font`); smart quotes,
  dashes and accents already work.
- [ ] **[P3 usability]** **Cancel a parade night entirely** once it has rows. Unticking Parade
  leaves the empty rows in the database (harmless, untidy).
- [ ] **[P4 nicety]** **PDF:** multi-week closures and weekend events aren't true merged
  cells, and long words break mid-word in the Weekends column. Compare
  against `examples/pdfs/` after any `internal/pdfgen` change (see the
  `pdf-fidelity` skill).
- [ ] **[P3 usability]** **Mobile layout.** Check uniform-swatch contrast. The grid scrolls sideways
  on a phone; a real mobile layout is still open. (Screen-reader and keyboard
  walk-throughs are not wanted for now; focus on usable features and bugs.)
- [ ] **[P3 usability]** **More site settings on the website.** The Email settings page
  (database over environment, encrypted password, test button) is built for mail only.
  Candidates for the same page: `ISSUE_EMAIL` and `ISSUE_MASKING` (they are read once
  at start-up today, so the report link needs a restart), the claim domain, and
  `DISABLE_SIGNUPS`. Also: a "mail not set up" mode that lets the master admin sign in
  and configure mail on a brand-new site without `ALLOW_LOG_MAIL`.

- [ ] **[P2 helpful feature]** **Compare two programme versions** side by side on the history page (today you can only restore).
- [ ] **[P3 usability]** **Phone layout for the programme grid.** The top bar and tables wrap instead of widening the page
  and a browser test (e2e/) guards that; the programme grid itself still scrolls sideways.
- [ ] **[P3 usability]** **Password quality:** a check against very common passwords (the 12-character
  minimum is the only rule today), and "last used" on the sessions list.
- [ ] **[P4 nicety]** **Squadron crest/logo image** in the PDF header. The website link is done;
  an image needs a first upload path (storage plus an upload form), which the
  app does not have yet.

## Planned improvements to existing screens

- [ ] **[P3 usability]** **Reorder rows within a night / requirement entries:** moving a night's
  activities to another date is done (drag or the Move control). Reordering
  requirement categories and entries is not; reuse the same pattern.
- [ ] **[P3 usability]** **Clearer classification grouping:** an explicit "these classes do the
  same thing" control instead of typing identical text to merge cells.

## Feature ideas

Roughly ordered by value for the effort.

1. **[P2 helpful feature]** **Instructors list, next step.** The list (name and rank, now editable) and
   display names exist. Still to do: per-instructor availability (see the duty rota idea below).
2. **[P2 helpful feature]** **Activities and training classes library.** A per-squadron catalogue
   (title, syllabus/class, default instructor, notes) to pick from when
   filling the grid. A proper searchable popup needs a small first-party
   script, which is now allowed (see the README's "Scripts" note); keep the
   form usable without it where reasonable.
3. **[P4 nicety]** **Ideas board** (reminder notes for future months). Each idea has a title,
   type, possible dates, contact details (e.g. RAF Careers), equipment needed,
   cost, status (idea / asked / confirmed / dropped) and a "use in programme"
   button that drops it into a day.
4. **[P4 nicety]** **QR-code suggestions from cadets.** Each squadron gets a revocable public
   link and QR code to a short form: type (camp, training day, airshow or
   airbase visit, trip, other), proposed dates, cost, transport, location,
   contact name, description. Submissions land in an inbox on the Ideas board
   for staff to accept or reject. It's a public write endpoint, so it needs:
   rate limiting (already have `ipLimiter`), a hidden honeypot field, strict
   length limits, no cadet names or phone numbers collected, and moderation
   before anything is shown.
5. **[P2 helpful feature]** **Read-only sharing:** a read-only link for
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
6. **[P2 helpful feature]** **Reminders:** email staff N days before a parade night that still has
   blank activity or instructor cells.
7. **[P2 helpful feature]** **Programme templates** for recurring evenings (e.g. "Monday: drill, then
   first aid").
8. **[P3 usability]** **Instructor availability and duty rota,** with a clash warning when the
   same person is on two things at once.
9. **[P3 usability]** **Audit trail:** "last edited by X at Y" on each programme, and an
    optional change log.
10. **[P3 usability]** **Documentation, next steps.** The versioned in-app docs exist
    (`/docs`, see `.claude/skills/docs/SKILL.md`). Still to do: add pages for
    each feature on this list as it is built; releases are cut by the pipeline on test (`devtools release`);
    the first one (1.9) exists, so the version selector and compare view have something to compare;
    link relevant screens to their help page; images/screenshots
    (not supported yet); in-docs search (a first-party script is now allowed).

### Missing features spotted along the way

Small gaps found while building and reviewing; each is a candidate to pick up.

- **[P3 usability]** **Edit uniform colours** (names and colours) in squadron settings; today only
  the five defaults exist.
- **[P3 usability]** **Resend an invite** (and see when it was sent); today you revoke and re-invite.
- **[P4 nicety]** **Print-friendly programme page** (the PDF covers most of this).
- **[P3 usability]** **Search** across programmes for an activity or a cadet's name.
- **[P4 nicety]** **Last-seen / last-edited** on the dashboard, to spot stale programmes.

## Reference

`examples/pdfs/` holds synthetic PDFs from `cmd/genexamples` (fabricated
data). Never commit a real squadron programme; real cadet and staff names were
scrubbed from this repo's history once already.

## Master admin: still to do

The account, the squadron switcher (read-only), the banner and the audit log
lines are built and tested. What is left, and why it is not done here:

- [x] **Issue reporting** (done: "Report a problem", emailed to `ISSUE_EMAIL`, not stored).
- [ ] **[P3 usability]** **Let the master admin act as an editor or owner** in a squadron when
  helping fix something (today it is strictly read-only, on purpose). If added,
  make it an explicit second step with its own audit line.
