---
title: Automated checks
order: 130
summary: What runs on every change, and what to do when one fails.
since: 1.0
audience: dev
changed:
  - next: Added the browser tests and a Releases section.
  - 1.9: Added staticcheck, gosec, CSS and HTML linting, workflow linting, secret and configuration scanning, fuzzing, operational tests and the documentation check.
---
# Automated checks

For people changing the code. Every push and pull request runs these in GitHub.

| Check | What it catches |
| --- | --- |
| Build, vet, test | Compile errors; the unit, integration and operational tests, run with the race detector. Coverage is shown in the run summary. |
| gofmt | Unformatted code. |
| staticcheck, gosec | Likely bugs and common security mistakes in Go code. |
| govulncheck | Known vulnerabilities in the Go libraries and standard library in use. |
| Trivy (source) | Vulnerable dependencies, secrets committed by mistake, and unsafe Docker settings. |
| CodeQL | Further security analysis of the Go code. |
| Stylelint | Mistakes in the CSS. |
| Browser tests | Playwright drives the real site in Chromium against a throw-away database: this month's programme, ticking Parade, moving a night by control and by drag and drop, closures, history and restore, the instructors page, phone widths and the report form. They live in `e2e/`. |
| HTML validation | Starts the site in-process and validates the pages it renders, including the programme editor, which catches things like a form nested inside a form. |
| Script rules | Fails on inline scripts, inline event handlers or `javascript:` links in the templates (the security policy refuses them). When the site has its own JavaScript or TypeScript, type-checks it strictly, and checks that committed compiled TypeScript is up to date. |
| actionlint | Mistakes in the workflow files. |
| Docs check | Embedded docs are valid, links resolve, and user-visible code changes come with a docs change. |
| Fuzzing | A short random-input run against the claim-email checker. |

On a push to the deployment branches the image is also built, started and
checked at `/health`, scanned for vulnerabilities, and published.

## Releases

Changes are promoted in order: `dev`, then `test`, then `main`, and each push must already have passed CI on the
stage before it (the promotion gate refuses anything else, and nothing is built or published).

- A push to `test` is the release. If the docs have `next` entries or `CHANGELOG.md` has an
  `## Unreleased` section with content, the pipeline freezes them as the next `1.<N>`, commits that to
  `test`, and merges the release commit back into `dev`. Do not run the snapshot tool by hand.
- A push to `main` keeps the version. `dev` builds show `1.<N>+<commit>`.
- Every build is started and checked at `/health`, scanned, and published to GitHub's container registry.

Keep writing `next` docs entries and `## Unreleased` changelog lines; see [about these docs](about-these-docs).

## The tests

- **Unit tests** per package.
- **Integration tests** run the whole site against a real temporary database:
  sign-in, roles, one squadron not seeing another's data, form tokens, rate
  limiting, security headers, escaping of user text, and the programme workflow.
- **Operational tests** build the real program and run it as a separate
  process: refusing bad settings, shutdown on signals, data surviving a restart,
  and many simultaneous requests.

Run everything with `go test ./...`. Add `-short` to skip the operational tests.

## When the docs check fails

You changed something users or operators can notice without changing the
documentation. Update the pages in `internal/docs/content/next/`: see
[about these docs](about-these-docs). The pull request description may instead
contain a line `No docs impact: <reason>` when that is true.
