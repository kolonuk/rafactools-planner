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
- [ ] **`DADOCK_DEPLOY_TOKEN` secret — still needed.** A fine-grained PAT,
  `Actions:write` on `kolonuk/dadock` only, set as a secret **on this repo**
  (`gh secret set DADOCK_DEPLOY_TOKEN --repo kolonuk/rafactools-planner`).
  dadock's side (compose service, homepage entry, `RAFACTOOLS_SESSION_KEY`
  slot) is already wired and pushed — this token is the one missing piece
  stopping a push to `main` from actually reaching the running stack. Create
  it at github.com/settings/personal-access-tokens/new (fine-grained,
  repository access limited to `kolonuk/dadock`, permission `Actions: write`),
  or reuse the same PAT already used as `sbcwaste.top`'s/`johnwood.dev`'s
  `DADOCK_DEPLOY_TOKEN` if you still have that value. Until this is set,
  `deploy.yml` pushes the image to GHCR and stops, printing a reminder.
- [ ] **Trigger the first real deploy.** Once the token above is set, either
  push to `main` again or `gh workflow run "1. Deploy docker stack" --repo
  kolonuk/dadock` to pick up the service for the first time (it needs
  `docker compose up -d` to actually run — the compose/homepage/env.template
  changes landing in git don't do that by themselves).

## Three real environments

One real deployment target exists now: prod, wired into dadock (see
`dadock`'s `docker-compose.yml` `rafactools` service, `config/homepage/
services.yaml`, `config/env.template`, `scripts/bootstrap.sh` — all pushed).
It's LAN-only (`http://<dadock-host>/rafactools/`, no TLS, no public domain)
since there's no domain yet — see below. `dev`/`test` pushes still only get
as far as a tagged image in GHCR (`dev-<sha>`/`test-<sha>`) — nothing pulls
and runs them. To make those real too:

- Decide where they run. Options: more containers in the same dadock compose
  stack on different paths (cheapest, matches the pattern just added for
  prod — copy the `rafactools` service, rename, pick a different
  `PathPrefix`), or genuinely separate hosts.
- Add a second/third `trigger-dadock-deploy`-style job in `deploy.yml` for
  the `dev`/`test` branches once those targets exist.
- Each one needs its own `SESSION_KEY` secret and its own data directory —
  don't point two environments at the same SQLite file.

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
- [ ] `AddHalf` only grows a parade night from 1 half to 2 — there's no UI to
  shrink it back, or to cancel a parade night that already has rows without
  leaving orphaned empty rows in the DB (harmless, just untidy).
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

## Example PDFs

`examples/pdfs/` holds synthetic PDFs generated by `cmd/genexamples` —
fabricated squadron/cadet/staff data, real layout patterns. If you're ever
handed a *real* squadron programme again to diagnose a layout issue, don't
commit it — see `.claude/skills/pdf-fidelity` for what to do instead (the
real examples this was originally built from carried real cadets'/staff's
names and were deliberately scrubbed from this repo and its git history).
