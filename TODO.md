# TODO

## Before this is real infrastructure anyone relies on

- [ ] **Password reset.** Not implemented at all — a locked-out user has no
  self-serve recovery. Needs a token flow mirroring email verification
  (`internal/auth`: `email_verifications` table/pattern already there to copy).
- [ ] **Real SMTP provider.** Right now `SMTP_HOST` unset just logs links to
  stdout (`internal/mail`). Fine for dev; pick a real relay (a transactional
  provider, or an existing dadock-reachable SMTP relay if one exists) for
  test/prod and set `SMTP_HOST`/`SMTP_USER`/`SMTP_PASS`/`MAIL_FROM` there.
- [x] **`SESSION_KEY` as a real secret (prod).** `dadock/config/env.template`
  has `RAFACTOOLS_SESSION_KEY`; **pending**: the actual secret value needs
  setting — `gh secret set RAFACTOOLS_SESSION_KEY --repo kolonuk/dadock --body "$(openssl rand -hex 32)"`
  (this needed a human to run — it writes a secret, which the agent that did
  the rest of this wiring was blocked from doing itself).
- [ ] **GitHub branch protection.** Require `CI` to pass before merging into
  `test`/`main`. Not something this repo's files can configure — a GitHub
  repo setting once the repo exists there.
- [x] **`DADOCK_DEPLOY_TOKEN` secret.** Set on this repo (reusing dadock's
  own `GH_PAT`, verified first to actually have `Actions:write` on
  `kolonuk/dadock` before setting it).
- [x] **Trigger the first real deploy.** Done manually
  (`gh workflow run "1. Deploy docker stack" --repo kolonuk/dadock`) —
  future pushes to `main`/`dev`/`test` trigger it automatically.

## Three real environments

All three now run as separate containers in the same dadock compose stack
(`dadock`'s `docker-compose.yml`: `rafactools`, `rafactools-dev`,
`rafactools-test` services), each with its own `SESSION_KEY` secret and its
own data directory, on different `PathPrefix`es
(`/rafactools`, `/rafactools-dev`, `/rafactools-test`). All LAN-only, no
TLS, no public domain — see "No domain yet" below.

- **dev** (`rafactools-dev`): `SKIP_EMAIL_VERIFICATION=true` — claiming a
  squadron logs straight in, no email step, nothing sent anywhere. Its
  SQLite data directory gets wiped and recreated empty every night at ~2am
  (see dadock's "Reset dev database" scheduled workflow) — don't put
  anything in dev you want to keep.
- **test** (`rafactools-test`): currently behaves like a normal instance
  (signups allowed, real verification flow) — that's not the intended final
  state. **Intended design, not yet built:** test should *not* allow new
  signups via the claim form at all; instead, each deploy to test should
  take a snapshot of prod's live SQLite database and load it into test, so
  test is always testing against real-shaped data rather than accumulating
  its own throwaway squadrons. Needs: a way to disable the claim route (or
  have it 404/redirect) when some `DISABLE_SIGNUPS`-style flag is set, plus
  a snapshot-and-restore step in dadock's deploy pipeline for the
  `rafactools-test` service specifically (copy prod's `.db` file into test's
  data directory, probably as part of "1. Deploy docker stack" or a
  dedicated step, *before* starting/restarting the test container).
- **prod** (`rafactools`): real squadron data, real email verification,
  unchanged.

## No domain yet

Local dev (`docker-compose.yml` at this repo's root) and the CI smoke test
both use `http://localhost:8080`, which is fine since nothing else needs to
reach them. **Prod's `BASE_URL`** is `http://<dadock HOST_IP>/rafactools`
(set in dadock's compose) — real but LAN-only and unencrypted, since there's
no domain yet. Once a domain exists: get it behind TLS (dadock's Traefik
already serves everything over `websecure` with a self-signed cert as a
fallback — a real domain + Pangolin tunnel replaces that), update
`BASE_URL` to the real `https://` address in dadock's compose, and flip
`COOKIE_SECURE` to `"true"` there (it's `"false"` right now specifically
because the LAN path is plain HTTP).

## PDF layout fidelity

Single portrait page, with every row height computed from actual wrapped
line counts (no more fixed-height/landscape/multi-page — that was the v1
approach and produced both a 2-page export and a real text-overlap bug,
both now fixed). Remaining gaps tracked in detail in
`.claude/skills/pdf-fidelity/SKILL.md`: multi-week closure banners and
cross-week weekend events aren't true rowspans yet, and `SplitText`'s
word-wrap breaks long words mid-word in the narrow Weekends column (no
hyphenation). Re-check against `examples/pdfs/` after any change to
`internal/pdfgen`.

## Smaller gaps

- [ ] No automated tests yet (`internal/auth`, `internal/programme`,
  `internal/pdfgen` are all hand-smoke-tested, not unit tested). `go test`
  runs in CI but there's nothing for it to run.
- [ ] `AddHalf` only grows a parade night from 1 half to 2 — needs three more
  things: (a) a "remove second half" action, which should warn first if
  half 2's content actually differs from half 1's (silently discarding
  different content is the dangerous case; dropping an identical duplicate
  isn't); (b) when *adding* a second half, clone half 1's activity/
  instructor text into it as a starting point instead of leaving it empty —
  the common case is both halves being the same or similar, not blank; (c)
  no way to cancel a parade night that already has rows without leaving
  orphaned empty rows in the DB (harmless, just untidy).
- [ ] No squadron crest/logo image in the PDF header, and no way to link out
  to the squadron's own website/social page from the app either —
  `pdfgen.go` renders text only, and there's no per-squadron "badge or
  external link" field anywhere. The real programmes have the RAFAC
  roundel + squadron crest. Needs: an upload (or URL) field in squadron
  settings, storage (a blob column or a file on disk — this app currently
  has no file upload path at all, so this is also the first thing that'd
  need one), and embedding the image in `pdfgen.go`'s title band.
- [ ] Grid editor (`programme_edit.html`) isn't tested on mobile widths — it's
  a wide table, likely needs horizontal scroll at minimum on a phone.
- [ ] `programme_requirement_categories`/`entries` have a `sort_order` column
  that's only ever appended to — no drag-reorder UI.
- [ ] No accessibility pass (focus states, ARIA labels on the icon-only
  delete/revoke buttons, colour-contrast check on the uniform-key swatches)
  beyond what the Material color tokens give for free.
- [ ] **In progress, paused mid-build:** a separate instructors list for the
  grid's Instructor row — prefilled options for "All Staff", each
  registered planner user (per squadron), or a custom typed name. Plan: an
  HTML `<datalist>` (no JS needed — a `list=` attribute on the existing text
  input, browser-native autocomplete, typing anything not listed still
  works) built from `auth.Service.ListMembers`, attached only to Instructor
  sub-rows (not Activity) in `programme_edit.html`. Not started: fetching
  members in `programmeEdit`'s handler, the `<datalist>` markup itself, and
  the `list=` attribute on the right inputs. Note in passing: `users.
  display_name` exists in the schema but nothing ever sets it, so "each
  registered planner user" would show as their email — fine for v1, but a
  display-name field in squadron settings would make this much more
  readable before relying on it.
- [ ] fpdf's core Helvetica font is WinAnsi/Latin-1, not UTF-8 — any
  non-Latin-1 character (an em-dash, a smart quote, a squadron name with an
  accented character) reaching `internal/pdfgen` renders as mojibake rather
  than erroring. Found via the version-footer's em-dash (fixed there by
  just using a plain hyphen); not fixed project-wide. Proper fix is an
  embedded Unicode TTF via `Fpdf.AddUTF8Font`.

## Requested features (not yet built)

- [ ] **Copy previous month's notes/requirements.** A button on the "new
  month" flow (or on an existing draft programme) to pull the prior month's
  `programme_weekend_notes`-adjacent requirement categories and/or notes
  forward as a starting point, instead of every month starting from a
  structurally-blank requirement tracker. Needs deciding exactly what
  copies (requirement *categories* probably yes, *entries*/cadet names
  probably no — those are presumably different cadets each month) —
  worth confirming with whoever actually uses this before building it.
- [ ] **Drag-to-reorder activity rows**, to rearrange a month's parade
  nights without retyping. Tension to resolve first: this app currently
  ships `script-src 'none'` in its CSP (see `internal/web/web.go`
  `securityHeaders`) specifically because there's no JS anywhere — drag-
  and-drop fundamentally needs JS (HTML5 drag events, or a library). Either
  this is the feature that finally justifies loosening that CSP line (with
  a `script-src 'self'` and a small first-party script, no CDN), or it's
  worth asking whether "reorder" actually means "change which *date* an
  activity is on" (which the existing form fields can already do, just
  clunkily — editing a day's uniform/notes/rows already targets that day's
  DB row regardless of calendar order) before building real drag-and-drop
  for what might be a smaller problem.
- [ ] **Explicit classification grouping**, e.g. "all but 1st Class do
  Drill, 1st Class do something else" as a deliberate choice rather than a
  side effect. Right now `pdfgen`'s merge (`mergeSegments`) and the grid's
  saved text are the same mechanism: typing identical text into adjacent
  classification cells is what makes them merge into one shared cell —
  there's no explicit "group these classifications together" control, just
  implicit merge-by-matching-text. Workable for simple cases but doesn't
  scale well past 2 groups and isn't obvious from the editing UI alone (you
  have to already know the merge-by-matching-text rule). A real fix
  probably means a small per-row grouping control (e.g. checkboxes or a
  drag-to-group picker across the classification columns) that then writes
  the matching/non-matching text to produce the same merge behaviour today
  reads implicitly.
- [ ] **Day-of-week should not be a free-text field.** `programme_days.
  day_label` is currently an editable `<input type="text" size="2">` in
  `programme_edit.html` (`day_{{.ID}}_label`) — nothing stops it from being
  wrong for the actual date, or from silently disagreeing with
  `pdfgen.groupIntoWeeks`'/`drawWeekendCell`'s own weekday computation
  (which uses the *real* `time.Weekday()` of `programme_days.date`, not
  this label). This bit for real once already, in `cmd/genexamples` (see
  git history around the "page-break bug" fix) — a hand-picked label that
  didn't match the real calendar weekday silently broke week-grouping and
  weekend-note matching. Fix: derive the displayed label from `date` alone
  (`defaultDayLabels` in `internal/programme/store.go` already has the
  M/Tu/W/T/F/Sa/Su mapping used at generation time — just use it at render
  time too instead of reading a stored, editable column) and drop the
  column/input entirely, or at minimum make the input read-only.
- [ ] **Verify PDF export always reflects the current month's saved state**
  — specifically Notes and the requirement tracker, not just the grid.
  `programmePDF` already calls `GetFullProgramme` fresh on every request
  (no caching layer anywhere in this app) and `pdfgen.Render` already
  reads `fp.Requirements`/day `.Notes`, so this *should* already be
  correct — but it's untested (see "no automated tests yet" above) and
  worth a regression test pinning it down explicitly, not just trusting
  the code reading. Also worth watching once the test-environment DB
  snapshot (see "Three real environments" above) exists: test's PDF export
  will only ever be as fresh as its last snapshot of prod, which is a
  different kind of "stale" than a caching bug and should be obvious to
  whoever's looking at test, not a surprise.

## Example PDFs

`examples/pdfs/` holds synthetic PDFs generated by `cmd/genexamples` —
fabricated squadron/cadet/staff data, real layout patterns. If you're ever
handed a *real* squadron programme again to diagnose a layout issue, don't
commit it — see `.claude/skills/pdf-fidelity` for what to do instead (the
real examples this was originally built from carried real cadets'/staff's
names and were deliberately scrubbed from this repo and its git history).
