---
name: lint
description: Run this repo's full local lint/build/test suite (actionlint for GitHub Actions workflow files, gofmt, go build/vet/test, govulncheck). Use before every `git commit` or `git push` to any branch — and always when a `.github/workflows/*.yml` file changed. Also use when explicitly asked to lint, preflight, or validate the repo.
---

# Local preflight checks

Run these, in order, and fix any failure before committing or pushing.

## 1. Workflow file lint (actionlint) — whenever `.github/workflows/*.yml` changed

```sh
docker run --rm -v "$PWD":/repo -w /repo rhysd/actionlint:latest -color
```

A plain YAML parser will accept a workflow GitHub's runner rejects outright
(e.g. `secrets` in a job-level `if:`). Only actionlint catches that before push.

## 2. gofmt

```sh
gofmt -l .
```

Any output here is an unformatted file — run `gofmt -w .` and re-check.

## 3. Go build, vet, test

No local Go toolchain is assumed to be installed — use the same image the
Dockerfile build stage uses:

```sh
docker run --rm -v "$PWD":/app -w /app golang:1.26-alpine sh -c \
  "go build ./... && go vet ./... && go test ./... -count=1"
```

(`-count=1` disables the test cache. Alpine has no C toolchain, so skip
`-race` here — CI's `ubuntu-24.04` runner has one and runs the race-enabled
test there; don't skip it in CI.)

## 3b. Static analysis and frontend lint (mirrors CI `go-lint` / `frontend-lint`)

```sh
go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.22.4 -severity medium -exclude-dir=cmd/genexamples ./...
npx --yes -p stylelint@16.10.0 -p stylelint-config-recommended@14.0.1 stylelint "internal/web/static/**/*.css"
go run ./cmd/devtools validate-html
```

## 3c. Docs

```sh
go test ./internal/docs ./internal/web -count=1   # validity, links, versions, routes
```

For a PR, also run `go run ./cmd/devtools docs-check` with `BASE`, `HEAD` (commit hashes) and
`PR_BODY` set (see `.claude/skills/docs/SKILL.md`).

## 4. Known-vulnerability scan

```sh
docker run --rm -v "$PWD":/app -w /app golang:1.26-alpine sh -c \
  "go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./..."
```

## 5. Full image build + smoke test

Before pushing a change that touches `cmd/`, `internal/`, or the Dockerfile,
build the real production image and hit `/health` — this is what `deploy.yml`
does in CI, and it has caught issues (e.g. a missing writable `/data` dir on
the distroless final stage) that `go build` alone did not:

```sh
docker build -t rafactools-planner:dev .
docker run -d --name rt-lint -p 18099:8080 \
  -e SESSION_KEY=lint-smoke-test-session-key-0123456789 -e COOKIE_SECURE=false \
  rafactools-planner:dev
sleep 1 && curl -sf http://localhost:18099/health
docker rm -f rt-lint
```

## Notes

- CodeQL and the Trivy filesystem/image scans also run in CI but need more
  setup locally — CI is the backstop for those.
- This applies to every branch (dev/test/main), not just main.

## Scripts (only when `internal/web/static/js/` or `internal/web/ts/` exist)

```sh
npx --yes -p typescript@5.6.3 tsc -p tsconfig.json         # strict type-check of JS (JSDoc) and TS
npx --yes -p typescript@5.6.3 tsc -p tsconfig.build.json   # compile TS into static/js/gen, then:
git diff --exit-code -- internal/web/static/js/gen         # committed output must be current
```

The CSP is `script-src 'self'`: no inline `<script>`, no inline `on…=` handlers, no `javascript:` URLs.
