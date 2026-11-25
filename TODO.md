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


- [ ] **[P4 nicety]** **Public-repo tidy-up.** Re-read the in-app help pages (`run-your-own`, `about-these-docs`)
  and the `.claude/skills` notes for wording that assumes a private project or a particular
  host, and make them neutral. Edit the `next` docs; released snapshots stay as published.

- [ ] **[P2 helpful feature]** **Planner redesign and public sharing: the plan, in order.** Decided: no Bootstrap;
  keep the hand-written Material-style CSS. One phase per pull request through dev, test and main.
  1. *Foundations:* DONE. Theme toggle (system first), breadcrumb and the Actions menu with the parade
     switch as a tick or cross. Still to do: phone layout of the top bar.
  2. *The grid:* the restyle is DONE (flowing list, quiet boxes, uniform colour dot, cards on a phone;
     still a table underneath, so the forms, drag and tests are unchanged). Still to do: a true
     day-per-card layout rather than a restyled table, the "Day" column after a move, and inline
     edit (tick/cross), which arrives with the same component.
  3. *Sharing settings:* DONE. Parade start and end time in squadron settings (default 19:00 to 21:30,
     Europe/London), a Published setting on each month (Actions menu) and a per-squadron public link
     that expires after 1 month and can be renewed.
  4. *Public files:* DONE. The PDF (no requirement tracker) and a PNG of each published month at the
     same link and expiry. The PNG is drawn separately in Go (`internal/pngrender`), because the
     image is distroless and cannot run a PDF-to-image tool; it needs to be kept in step with the PDF
     by eye. Still to do: line the PNG layout up with the PDF more closely.
  5. *Calendar feed (`.ics`):* DONE (one event per parade night from today, uniform and grouped
     activities, link to the month's PDF, weekend notes as all-day entries, closures left out,
     expired link answers 410, a night taken out after it was shown goes out as cancelled; instructor
     names are deliberately never included).
  6. *QR code* for the feed at the foot of the PDF and the PNG, with its expiry date: DONE. Still to
     do: the same on a public web page (there is none yet).
- [ ] **[P3 usability]** **More detail in the programme history.** The history page should say what
  actually changed, not just that a version was saved. For example: "Moved 3 Nov activities to
  10 Nov", "Added closure 22-26 Dec", "Changed uniform for 17 Nov from Blue to Working dress",
  "Edited instructor on 10 Nov (First Class)". Work out the changes by comparing the saved
  version with the one before it, and use the same wording on the compare view when that exists
  (see "Compare two programme versions").
- [ ] **[P3 usability]** **PDF: the colour key overlaps the planner.** Fix the layout in
  `internal/pdfgen` so the key never sits on top of the grid; compare against `examples/pdfs/`
  (see the `pdf-fidelity` skill).
- [ ] **[P3 usability]** **Bug: moving a day in the grid does not update the "Day" column.** After a
  drag or the Move control, the weekday label keeps the old value until reload.
- [ ] **[P3 usability]** **Lift the JavaScript restriction and review reactive features.** The
  README still says "no JS framework" and "pages should still work without script where
  reasonable". Remove that restriction (keep the CSP `script-src 'self'`, no inline script, no
  third-party or `eval`), update the README, `docs` skill and lint skill wording, then review
  which screens would be better with TypeScript (live validation, inline edit, drag and drop,
  partial refresh) and add the worthwhile ones as items.

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

- [ ] **[P3 usability]** **Phone layout for the programme grid (top bar still to do).** The top bar and tables wrap instead of widening the page
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

1. **[P4 nicety]** **Ideas board** (reminder notes for future months). Each idea has a title,
   type, possible dates, contact details (e.g. RAF Careers), equipment needed,
   cost, status (idea / asked / confirmed / dropped) and a "use in programme"
   button that drops it into a day.
2. **[P4 nicety]** **QR-code suggestions from cadets.** Each squadron gets a revocable public
   link and QR code to a short form: type (camp, training day, airshow or
   airbase visit, trip, other), proposed dates, cost, transport, location,
   contact name, description. Submissions land in an inbox on the Ideas board
   for staff to accept or reject. It's a public write endpoint, so it needs:
   rate limiting (already have `ipLimiter`), a hidden honeypot field, strict
   length limits, no cadet names or phone numbers collected, and moderation
   before anything is shown.
3. **[P2 helpful feature]** **Read-only sharing, what is left.** The link, the calendar feed, the PDF and PNG
   and the QR code on them are built, and owners are emailed a week before the link ends. Still to do: a
   public web page for the squadron (it would show the published months and carry the QR code for the
   feed, with the expiry date), and the QR code for the cadet suggestion form (item 2).
4. **[P3 usability]** **Duty rota and clash warning:** time off is recorded and warned about in the grid;
   still to do is a warning when the same person is on two things at once.
5. **[P3 usability]** **Audit trail:** "last edited by X at Y" on each programme, and an
    optional change log.
6. **[P3 usability]** **Documentation, next steps.** The versioned in-app docs exist
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
