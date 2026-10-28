---
title: Automated checks
order: 130
summary: What runs on every change, and what to do when one fails.
since: 1.0
changed:
  - next: Added staticcheck, gosec, CSS and HTML linting, workflow linting, secret and configuration scanning, fuzzing, operational tests and the documentation check.
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
| HTML validation | Starts the site and validates the pages it renders. |
| Script rules | Fails on inline scripts, inline event handlers or `javascript:` links in the templates (the security policy refuses them). When the site has its own JavaScript or TypeScript, type-checks it strictly, and checks that committed compiled TypeScript is up to date. |
| actionlint | Mistakes in the workflow files. |
| Docs check | Embedded docs are valid, links resolve, and user-visible code changes come with a docs change. |
| Fuzzing | A short random-input run against the claim-email checker. |

On a push to the deployment branches the image is also built, started and
checked at `/health`, scanned for vulnerabilities, and published.

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
