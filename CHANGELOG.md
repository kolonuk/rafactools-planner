# Version history

Format: each entry is `1.<PR number>` — the GitHub PR that merged it (see
`.github/workflows/deploy.yml`, which computes this at build time and bakes
it into the running binary's footer/PDF-footer version string). Entries
grouped by type: **feat** (new capability), **fix** (bug), **chore**
(infra/tooling/docs, no behaviour change for users), **security**.

Changes before this file existed (everything up to and including the
version-footer feature itself) were pushed directly to `main`, not through
PRs, so they have no PR number to key off — summarized below as a single
baseline instead of per-entry.

## Unreleased

- **feat**: in-app help at `/docs`, behind login and in the app's theme: 14
  pages for users and operators, a version selector, a compare view and
  per-page "first appeared in / changed in" notes. Embedded in the binary;
  validated at build time and at startup. A "Help" link is in the top bar.
- **fix**: removing a member who had created a programme or sent an invite failed with a raw database error; their programmes now stay (without a creator), their pending invites are withdrawn, and unexpected errors no longer show database text.
- **security**: the squadron website address is validated on the server (http/https only) before it becomes a link on the dashboard and in the PDF.
- **fix**: the new "Remove second half" button sat inside the grid form like "Add a second half" did, so it submitted the grid instead; it now uses its own form.
- **feat**: password reset ("Forgot your password?"): an emailed link that works once and expires after an hour, stored only as a hash, never reveals whether an address has an account, is rate limited, and signs the user out everywhere when used. The handlers now send mail through a small `Mailer` interface so tests can follow the real links.
- **feat**: move a parade night's activities to another date, by drag-and-drop (TypeScript, `internal/web/ts/programme-grid.ts`, compiled into `static/js/gen/`) or with a plain "Move" control that works without scripts, with the keyboard and on touch screens. It swaps the two dates so nothing is overwritten, and saves other pending edits first.
- **fix**: "+ Add a second half to this night" did nothing in a real browser (a form nested in a form made it submit the whole grid); fixed, and the programme editor is now part of the rendered-HTML validation. Table headers have `scope` for screen readers.
- **fix**: passwords over 72 bytes were refused with a raw bcrypt error; they
  now get a clear message. Ticking "Parade" on a night that was not generated
  as one gave it no activity or instructor boxes; it now does. The
  check-your-email page no longer tells users to read TODO.md.
- **chore**: the browser policy now allows the site's own JavaScript/TypeScript (`script-src 'self'`), still refusing inline, third-party and eval scripts; CI type-checks scripts with `tsc` instead of rejecting them.
- **chore**: docs skill, docs snapshot script and a pull-request check that
  user-visible changes include a docs change.

- **security**: requirement entries could be added to or deleted from another
  squadron's programme by forging a category ID; mail headers could be
  injected via invite addresses/squadron names; rate limiter trusted a
  spoofable `X-Forwarded-For`; SMTP TLS had no minimum version; login timing
  revealed which emails exist; `SESSION_KEY` must now be 32+ characters.
- **fix**: a closure with unparseable dates made PDF export loop forever;
  closure and weekend-note dates are now validated. Unknown URLs returned the
  dashboard instead of 404. Parade weekdays outside 1-7 and blank squadron
  names are no longer saved.
- **chore**: unit, integration, fuzz and operational (real-binary) tests;
  staticcheck, gosec, stylelint, rendered-HTML validation, actionlint, Trivy
  secret/misconfig scanning and SHA-pinned actions in CI.

- **feat**: parade-evenings-only toggle on the programme grid (CSS-only
  checkbox, hides non-parade filler rows).
- **feat**: build version shown in the web footer and PDF footer, linking
  here — `internal/version`, wired through `Dockerfile`'s `VERSION` build
  arg and `deploy.yml`'s PR-number lookup.
- **fix**: PDF footer text used an em-dash, which fpdf's core Helvetica font
  (WinAnsi/Latin-1, not UTF-8) rendered as mojibake — same trap applies to
  any other non-Latin-1 character reaching the PDF renderer.
- **feat**: instructor `<datalist>` on the grid's Instructor row — "All
  Staff" plus each registered squadron member, native browser autocomplete
  (no JS), still free text for anything else.
- **feat**: squadron members can now be removed or have their role changed
  from squadron settings — previously an owner could invite people but
  never demote or remove an existing member. Refuses to remove or demote
  the squadron's last owner.
- **feat**: squadron website link — set in squadron settings, shown on the
  dashboard and in the PDF title band.
- **feat**: adding a parade night's second half now clones the first
  half's text instead of leaving it blank; removing a second half warns
  (with a side-by-side comparison) if its content actually differs from
  the first half's, rather than silently discarding it.
- **fix**: a parade night's day-of-week letter was a free-text field a user
  could edit to disagree with the actual calendar date, which had already
  caused a real bug once (see `cmd/genexamples` history). Now derived once
  from the date at month-creation time and never user-editable.
- **fix**: programme grid wrapped in a horizontal-scroll container for
  narrow screens, instead of squeezing every column illegibly thin.
- **chore**: tests for the last-owner removal/demotion guard, that
  migrations apply exactly once, and that PDF output actually changes when
  the underlying Notes/requirement data does.

## Baseline (1.0) — initial build

**feat**
- Multi-tenant squadron model: claim a squadron via an `snco.<number>@`/
  `oc.<number>@` RAFAC address with email confirmation; invite-based
  membership (any email) for everyone else, with owner/editor/viewer roles.
- Monthly training-programme grid: parade nights (1–2 halves), per-
  classification or shared activity/instructor text, closures, weekend
  notes, footer requirement tracker — configurable classifications, uniform
  colours, and parade weekday(s) per squadron.
- PDF export matching the real paper format: single portrait page, row
  heights computed from actual wrapped line counts rather than fixed,
  closures/merged cells, uniform-key legend, requirement tracker.
- `SKIP_EMAIL_VERIFICATION` dev-mode flag: claiming a squadron logs straight
  in, nothing emailed — for the `dev` environment only.
- Three environments (`dev`/`test`/`main` branches), each deployed as its
  own container on the dadock stack with its own data and session-key
  secret; `dev`'s database resets nightly.

**security**
- Strict CSP (`script-src 'none'` — no JS anywhere in this app), security
  headers on every response, per-IP rate limiting on login/claim/invite-
  accept, 1MB request body cap, constant-time CSRF comparisons, no
  third-party requests (dropped Google Fonts — system font stack instead).

**chore**
- CI (build/vet/test/gofmt/govulncheck/Trivy) and a build-scan-push-to-GHCR
  deploy workflow, with a dadock redeploy dispatch on `main`.
- Synthetic example PDFs (`cmd/genexamples`) — the original examples this
  was modelled on carried real cadets'/staff's names and were deliberately
  kept out of this repo and its git history.
