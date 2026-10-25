# TODO

## Before this is real infrastructure anyone relies on

- [ ] **Password reset.** Not implemented at all — a locked-out user has no
  self-serve recovery. Needs a token flow mirroring email verification
  (`internal/auth`: `email_verifications` table/pattern already there to copy).
- [ ] **Real SMTP provider.** Right now `SMTP_HOST` unset just logs links to
  stdout (`internal/mail`). Fine for dev; pick a real relay (a transactional
  provider, or an existing dadock-reachable SMTP relay if one exists) for
  test/prod and set `SMTP_HOST`/`SMTP_USER`/`SMTP_PASS`/`MAIL_FROM` there.
- [ ] **`SESSION_KEY` as a real secret.** Must be a random 32+ byte value set
  per environment (dev/test/prod each need their own), not committed
  anywhere. Rotating it logs every session out.
- [ ] **GitHub branch protection.** Require `CI` to pass before merging into
  `test`/`main`. Not something this repo's files can configure — a GitHub
  repo setting once the repo exists there.
- [ ] **`DADOCK_DEPLOY_TOKEN` secret.** Set on this repo (fine-grained PAT,
  `Actions:write` on `kolonuk/dadock` only) so pushes to `main` actually
  trigger a redeploy. Without it, `deploy.yml` pushes the image to GHCR and
  stops, printing a reminder.

## Three real environments

Right now there's one real deployment target (prod, on dadock) and `dev`/
`test` pushes only get as far as a tagged image in GHCR — nothing pulls and
runs them. To make `dev`/`test` real:

- Decide where they run. Options: separate containers in the same dadock
  compose stack on different ports/subdomains (cheapest, matches how dadock
  already runs things), or genuinely separate hosts.
- Add the corresponding service block + Pangolin blueprint entry in the
  `dadock` repo (**a separate, shared-infra repo — don't push there without
  checking first**, even though its own convention is "always push, no PRs").
- Add a second/third `trigger-dadock-deploy`-style job in `deploy.yml` for
  the `dev`/`test` branches once those targets exist.
- Each environment needs its own `SESSION_KEY`, `SQLITE` data volume, and
  (once it exists) its own subdomain — see "No domain yet" below.

## No domain yet

`BASE_URL` currently points at `http://localhost:8080` wherever it's deployed
— verification/invite links will be wrong on any host other than the
operator's own machine until this is set to a real reachable URL per
environment. Once a domain exists: set `BASE_URL`, put it behind TLS (dadock's
existing Pangolin/Caddy setup presumably handles this — see
`dadock-vps-invariants` conventions), and set `COOKIE_SECURE=true` (already
the default — just don't override it to false outside dev).

## PDF layout fidelity

Tracked in detail in `.claude/skills/pdf-fidelity/SKILL.md`. Short version:
multi-week closure banners and cross-week weekend events aren't true rowspans
yet, and row height doesn't grow for long wrapped Notes text. Re-check against
`examples/pdfs/` after any change to `internal/pdfgen`.

## Smaller gaps

- [ ] No automated tests yet (`internal/auth`, `internal/programme`,
  `internal/pdfgen` are all hand-smoke-tested, not unit tested). `go test`
  runs in CI but there's nothing for it to run.
- [ ] `AddHalf` only grows a parade night from 1 half to 2 — there's no UI to
  shrink it back, or to cancel a parade night that already has rows without
  leaving orphaned empty rows in the DB (harmless, just untidy).
- [ ] No rate limiting on `/login` or `/claim` — fine at current scale
  (a handful of squadrons), worth adding (even a simple in-memory per-IP
  limiter) before this is internet-reachable at any real scale.
- [ ] No squadron crest/logo image in the PDF header — `pdfgen.go` renders
  text only. The real programmes have the RAFAC roundel + squadron crest;
  adding per-squadron logo upload + embedding is follow-up work.
- [ ] Grid editor (`programme_edit.html`) isn't tested on mobile widths — it's
  a wide table, likely needs horizontal scroll at minimum on a phone.
- [ ] `programme_requirement_categories`/`entries` have a `sort_order` column
  that's only ever appended to — no drag-reorder UI.
- [ ] No accessibility pass (focus states, ARIA labels on the icon-only
  delete/revoke buttons, colour-contrast check on the uniform-key swatches)
  beyond what the Material color tokens give for free.

## Where to drop new example PDFs

`examples/pdfs/` — already has ten real months plus two blank templates
across both the 4-classification and 5-classification ("Master" added)
template generations. More examples (especially ones showing a layout
variant not yet covered — different closure shapes, more than 2 halves,
different weekend-note spanning) are useful for `.claude/skills/pdf-fidelity`
work.
