# Contributing

Thanks for helping. This page is for people who want to change the code. For
what the app does, see the [README](README.md).

## How it is built

- **Backend**: Go (`cmd/server`, `internal/*`), no web framework. It uses the
  standard library `net/http` with Go 1.22+ method and pattern routing.
- **Storage**: SQLite (`modernc.org/sqlite`, pure Go, no CGO), one file kept on
  a volume. Migrations are embedded SQL files applied on start-up
  (`internal/db`).
- **Sign-in**: email and password (bcrypt), server-side sessions in the
  database with an HTTP-only cookie, and CSRF protection. See `internal/auth`.
- **Email**: SMTP (`internal/mail`) for verification, invite and password-reset
  emails. With no SMTP configured, links are logged to the console instead.
  That is fine for development and not for a real site.
- **PDF export**: `internal/pdfgen`, using `github.com/go-pdf/fpdf`. It is
  checked against the made-up examples in `examples/pdfs/` (regenerate them with
  `go run ./cmd/genexamples`). See `.claude/skills/pdf-fidelity` before changing
  it.
- **Help pages**: `internal/docs` renders embedded, versioned Markdown
  (`goldmark`, raw HTML disabled) inside the app layout. The pages are in
  `internal/docs/content/`; the authoring rules are in
  `.claude/skills/docs/SKILL.md`.
- **Front end**: server-rendered HTML in `internal/web/templates`, styled by a
  hand-written Material Design 3 theme (`internal/web/static/css/app.css`) that
  follows the system light or dark preference. Scripts are first-party only,
  under `internal/web/static/js/`. The content security policy is
  `script-src 'self'`, so no inline scripts or handlers and no third-party or
  `eval` scripts. TypeScript sources live in `internal/web/ts/` and compile into
  `internal/web/static/js/gen/` (generated; do not edit it; the compiled output
  is committed and CI checks that it is current). `tsconfig.json` type-checks
  both strictly.

## Running it locally

```sh
cp .env.template .env      # fill in SESSION_KEY at minimum
docker compose up --build
```

The app is on <http://localhost:8080> and Mailpit, which catches outgoing mail,
is on <http://localhost:8025>.

## Checks to run before you commit

The `lint` skill (`.claude/skills/lint/SKILL.md`) lists the full set: actionlint
for workflow files, `gofmt`, `go build`, `go vet`, `go test` and `govulncheck`.
Browser tests are in `e2e/`.

## Branches and releases

There are three branches, `dev`, `test` and `main`, promoted in that order by
pull request. Every push or pull request runs `.github/workflows/ci.yml`: build,
vet, tests, formatting, `govulncheck`, a Trivy scan and CodeQL.

After the checks pass, a push also runs a promotion-order gate and builds the
container image. The image is smoke-tested against `/health`, scanned with
Trivy and pushed to GHCR with an immutable `<branch>-<short-sha>` tag (there is
no `latest` tag).

This repository stops at the image. Running it (the host, TLS, secrets such as
`SESSION_KEY`, redeploying) is up to whoever hosts it. The "Run your own copy"
help page lists what the image needs.

## Repository layout

```
cmd/server/          the app's entry point
cmd/devtools/        release, docs and GitHub-setup helpers
cmd/genexamples/     regenerates examples/pdfs/ from made-up data
internal/
  auth/              sign-up, invites, sign-in, sessions, CSRF
  config/            settings read from the environment
  db/                SQLite and the embedded migrations
  docs/              the in-app help pages and their content
  mail/              SMTP sender
  programme/         the programme model and its storage
  pdfgen/            PDF rendering
  web/               routes, handlers, templates, static files
examples/pdfs/       example programmes (made-up data)
e2e/                 browser tests
.claude/skills/      working notes for lint, docs and PDF checks
```

## Data you must never commit

Never commit a real squadron programme, or real cadet or staff names. The
example PDFs are made up for this reason, and real names were removed from this
repository's history once already.

## What is planned

See [TODO.md](TODO.md).
