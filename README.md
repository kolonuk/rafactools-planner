# RAFACTools Planner

A monthly training-programme planner for an air cadet squadron (or any youth
group running a similar parade-night format), built to recreate the paper
programme format in `examples/pdfs/` as a shared, editable web app instead of
a spreadsheet one person maintains.

## What it does

- Multi-tenant: one deployment, many squadrons. The first person to sign up
  with an `snco.<number>@` or `oc.<number>@` address at the configured RAFAC
  domain **claims** that squadron number and becomes its owner. Everyone else
  joins by invite (any email address) from an existing member.
- Each squadron configures its own classifications (progression stages, e.g.
  Junior → 1st Class → Leading → Senior), parade weekday(s) (1–7 nights/week),
  and uniform colour key.
- Build a month's programme in a plain HTML grid (date, parade night half,
  activity/instructor per classification, notes, weekend events, closures,
  and a footer requirement tracker), then export it as a PDF matching the
  existing paper format.

## Architecture

- **Backend**: Go (`cmd/server`, `internal/*`), no web framework — stdlib
  `net/http` with Go 1.22+ method+pattern routing.
- **Storage**: SQLite (`modernc.org/sqlite`, pure Go, no CGO) — one file,
  mounted as a volume. Migrations are embedded SQL files applied on startup
  (`internal/db`).
- **Auth**: email+password, bcrypt, server-side sessions (DB-backed, HTTP-only
  cookie), CSRF via a session-derived token (or a double-submit cookie for the
  pre-login claim/login/invite-accept forms). No OAuth/SSO — see `internal/auth`.
- **Email**: SMTP (`internal/mail`) for the claim-verification and invite
  emails. With no SMTP configured, links are logged to stdout instead — fine
  for dev, never acceptable for test/prod (see TODO.md).
- **PDF export**: `internal/pdfgen`, using `github.com/go-pdf/fpdf`. Built
  from, and best effort-matched against, the real examples in
  `examples/pdfs/` — see `.claude/skills/pdf-fidelity` before touching it.
- **Frontend**: server-rendered HTML forms (`internal/web/templates`), no JS
  framework. Styled with a hand-rolled Material Design 3 theme
  (`internal/web/static/css/app.css`) that follows the system light/dark
  preference automatically.

## Local development

```sh
cp .env.template .env   # fill in SESSION_KEY at minimum
docker compose up --build
```

Opens the app on http://localhost:8080 and Mailpit (catches outgoing mail —
read the verification/invite emails there) on http://localhost:8025.

## Environments and deployment

Three branches: `dev` → `test` → `main`, promoted by PR. Every push/PR to any
of them runs `.github/workflows/ci.yml` (build, vet, test, gofmt, govulncheck,
Trivy filesystem scan, CodeQL). Every push to any of them also runs
`.github/workflows/deploy.yml`, which builds the image, smoke-tests it against
`/health`, scans it with Trivy, and pushes it to GHCR tagged
`<branch>-<short-sha>` (plus a rolling `<branch>-latest`).

**Only `main`** additionally dispatches `kolonuk/dadock`'s `"1. Deploy docker
stack"` workflow (via the `DADOCK_DEPLOY_TOKEN` repo secret — same pattern as
`sbcwaste.top`/`johnwood.dev`) so the running dadock stack picks up the new
image. There's no separate `dev`/`test` *host* yet — see TODO.md "Three real
environments" for what that actually needs.

GitHub branch protection (require the CI workflow to pass before merging to
`test`/`main`) is **not** configured by this repo — that's a repo setting, do
it in GitHub once the repo exists there.

## Repo layout

```
cmd/server/          entrypoint
internal/
  auth/               claim/invite/login/session/CSRF
  config/             env var loading
  db/                 sqlite open + embedded migrations
  mail/               SMTP sender
  programme/          domain model + store (CRUD)
  pdfgen/             PDF rendering
  web/                routes, handlers, templates, static assets
migrations/           (embedded under internal/db/migrations)
examples/pdfs/        real example programmes this was built from
.claude/skills/       lint and pdf-fidelity skills for working in this repo
```

## Known gaps

See `TODO.md`.
