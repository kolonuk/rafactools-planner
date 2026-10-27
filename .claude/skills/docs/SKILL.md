---
name: docs
description: Use for ANY change to this repo that a user, squadron admin or operator could notice (screens, forms, behaviour, limits, config/env vars, roles, emails, PDF output, security behaviour, deploy/CI), and whenever asked to write, change or review documentation. Strictest skill in this repo after security — docs are part of the change, not a follow-up. Load it before committing or declaring any such work done.
---

# Documentation rules

**Priority order for this repo: security first, documentation second, everything
else after.** If a documentation rule and a convenience conflict, the
documentation rule wins. If a documentation rule and a security rule conflict,
security wins — and the conflict itself gets documented.

Documentation is not a follow-up task. A user-visible change without its docs
in the same commit is an unfinished change. Do not say "done", commit, push or
open a PR while any rule below is unmet.

## What the docs are

Markdown pages embedded in the binary and served at `/docs` **behind login**, in
the app's own theme (same layout, same CSS tokens), by `internal/docs` and
`internal/web/handlers_docs.go`. They are versioned with the app so a reader on
version X can read the docs as they were at X and compare with any other version.

```
internal/docs/content/
  next/            the working copy: edited in place, shown as "Unreleased"
  1.42/            frozen snapshot taken at release 1.42 (version = 1.<PR number>)
  ...
```

Pages are `<slug>.md` with front matter (see below). `next/` always describes the
code on the current branch — nothing more, nothing less.

### Why a custom renderer and not Hugo / MkDocs / mdBook / Docusaurus

Decided once; do not relitigate without a reason that beats these:

- Docs must sit **behind the app's login**, in the **same theme**, linking both
  ways with the app. A separate static site means separate hosting, separate auth
  and a second theme to keep in sync.
- The CSP is `script-src 'none'` and the image is a single static binary with
  embedded assets. Off-the-shelf generators lean on JavaScript (search, version
  switchers, navigation) and need their own build toolchain in CI and the image.
- Version selector, compare and change notes are all small server-side features
  (Markdown → HTML with `goldmark`, a line diff). Writing them is less work than
  bending a generator to do them under this CSP.
- `goldmark` is used with raw HTML **disabled** (its default), so a page can never
  inject markup or script.

## Rules — every one is mandatory

1. **Same commit.** Any change a user, admin or operator can notice updates
   `internal/docs/content/next/` in the same commit. This includes: new/changed/
   removed screens, fields, buttons, routes; roles and permissions; limits and
   validation (password length, date ranges, rate limits, body size); expiry and
   retention; emails sent; PDF content; env vars and defaults; startup failures;
   deploy/CI behaviour. No "docs later".
2. **Only what exists.** Never document a feature that is not merged and working.
   Roadmap items live in `TODO.md`, never in `docs/`. If behaviour is removed,
   delete or rewrite the page text in the same commit — a page describing
   behaviour that no longer exists is a bug.
3. **Security behaviour is always documented**, accurately and without
   exaggeration: what is stored, for how long, who can see it, what is rate
   limited, what expires, what a leaked link/token allows. Never claim a
   protection the code does not provide. When security code changes, re-read the
   matching page line by line.
4. **Front matter on every page**, strictly this shape (the parser rejects anything
   else, and `go test ./internal/docs` fails):

   ```
   ---
   title: Inviting people
   order: 20
   summary: One line shown in the page list.
   since: 1.0
   changed:
     - next: Invites to an address that already has an account are refused.
     - 1.42: Invites expire after 7 days.
   ---
   ```

   - `since` — the version the page's subject first existed in (`next` if new
     and unreleased).
   - `changed` — one entry per behaviour change *after* `since`, newest first,
     keyed by the version it arrived in. Written for a reader on an older
     version deciding "does this apply to me?". Cosmetic wording fixes are not
     changes; behaviour, limits, defaults and permissions are.
   - Versions are `next` or `1.<number>` only.
5. **Changelog too.** Every user-visible change also gets a line in
   `CHANGELOG.md` (type: feat / fix / security / chore). Docs and changelog must
   not contradict each other.
6. **Links are checked.** Link to other pages by slug (`[roles](roles)`), to the
   app with `app:/path` (e.g. `app:/squadron`), to the outside world with full
   `https://` URLs. Do not hand-write `/docs/...` or app paths — they break under
   a base path. Every page must be reachable from the docs index.
7. **Plain language.** Write for a busy squadron officer first and an operator
   second. Short sentences, concrete steps, say what the screen is actually
   called. No marketing words, no "simply" or "just". Name the role who can do
   each thing. Examples use fabricated data only — **never** real cadet, staff
   or squadron names, emails or numbers.
8. **Terms.** Use the app's own words: *squadron*, *programme*, *parade night*,
   *classification*, *closure*, *weekend note*, *requirements tracker*, *owner /
   editor / viewer*. Do not invent synonyms between pages.
9. **Verify before writing.** Read the code or run the app for every claim — do
   not write from memory or from `TODO.md`. A number in the docs (days, limits,
   characters) must be traced to the constant in code.
10. **Releases snapshot the docs.** When the owner designates a release
    (`1.<PR number>`), run `go run ./cmd/devtools docs-snapshot <version>` in that
    PR. It relabels `next` entries and `since: next` to the new version in both
    the working copy and the snapshot, copies `next/` to `<version>/`, and runs
    the docs tests. Read the snapshot in the running app, commit it, and never
    edit it afterwards except to fix a factual error or a security-relevant
    inaccuracy — and record that fix in its `changed:` list. A build whose
    version has no snapshot shows the newest docs instead.
11. **Done means checked.** Before saying done: run `go test ./internal/docs
    ./internal/web`, read the rendered page in the running app (or the test
    output) for each page you touched, and confirm the page list, version
    selector and compare view still work. If the docs check fails, the work is
    not done — fix it, never skip, weaken or delete the check.
12. **PRs.** CI (`go run ./cmd/devtools docs-check`) fails a PR that changes
    user-visible code (`internal/web`, `internal/config`, `internal/auth`,
    `internal/mail`, `internal/pdfgen`, `cmd/server`, `Dockerfile`,
    `docker-compose.yml`, workflows; test files excluded) without touching
    `internal/docs/content/`, unless the PR body has a line starting
    `No docs impact: ` followed by a specific reason. Do not write that line to
    get past the check; use it only when it is true, and say why. Locally you
    can run the same tool with `BASE`, `HEAD` (full commit hashes, e.g.
    `BASE=$(git merge-base origin/main HEAD) HEAD=$(git rev-parse HEAD)`) and
    `PR_BODY` set. All repo tooling is Go (`cmd/devtools`); no Python or shell
    scripts belong in the repo.
13. **Accuracy beats completeness.** If you cannot verify a statement, leave it
    out and say so; do not soften it into something vague. When writing or
    reviewing docs, test the claim against the running app or a test, as was done
    for the 72-character password limit and the "Parade" tick behaviour. If the
    docs reveal a bug, fix it or add it to `TODO.md` — and until fixed, document
    the real behaviour under `known-limits`, never an imagined one.

## Checklist (copy into your working notes for any user-visible change)

- [ ] Which pages describe this behaviour? Updated each, claims verified in code
- [ ] New page? Front matter complete, `order` set, linked from the index
- [ ] `since` / `changed` entries added with the right version label
- [ ] Security-relevant? Page states what is stored / expires / limited / exposed
- [ ] `CHANGELOG.md` line added
- [ ] `TODO.md` item removed or trimmed if this completes it
- [ ] Nothing in `docs/` describes an unbuilt feature
- [ ] `go test ./internal/docs ./internal/web` green; pages read in rendered form
