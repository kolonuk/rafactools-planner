---
name: promote
description: How to get a change onto main in this repo (dev, then test, then main, by pull request, with the promotion-order gate). Use whenever asked to commit and push, open or merge a PR, promote a branch, release, or when the "Promotion order" CI check fails.
---

# Getting a change to `main`

Changes travel one way: **`dev` → `test` → `main`**. Never push straight to `test` or `main`, and
never force-push (branch protection forbids it, and the history was rewritten once already).

## Steps

1. Work on `dev`. Run the `lint` skill first (it covers `gofmt`, build, vet, tests, actionlint).
   If a user-visible thing changed, the `docs` skill applies too.
2. Commit and `git push origin dev`. Wait for the CI run on that push to pass.
3. Open a PR **`dev` → `test`** (`gh pr create --base test --head dev`). Wait for every check
   (`gh pr checks <n>`; there are nine required ones, and "Build, vet, test" and "Browser tests" are
   the slow ones). Merge it with `gh pr merge <n> --merge`. Wait for the CI run on `test` to pass.
4. Open a PR **`test` → `main`** (`--base main --head test`), wait for checks, merge the same way.
   Wait for the CI run on `main`: it builds, scans and publishes the image.
5. **Cut the release (version history).** Every user-visible change needs a `CHANGELOG.md` line under
   `## Unreleased` and a docs `changed: - next:` note in the same commit. When a batch is on `dev` and its
   CI is green, run the manual workflow: `gh workflow run release.yml --ref dev`. It freezes the unreleased
   docs and changelog as the next `1.<N>` and opens a PR `release/1.<N>` into `test` (the promotion check
   accepts it because the release commit's parent is on `dev`). A PR opened by the workflow's own token does
   not start CI, so its required checks never report and it stays blocked: close it and reopen it
   (`gh pr close <n>; gh pr reopen <n>`), which starts them. Merge that PR like any other, wait for CI on
   `test`, then promote `test` to `main` as in step 4. Do not skip this: the version shown in the footer and
   PDF, and the "Version history" page, only move when a release is cut.
6. After the merges, `git fetch` and fast-forward local `dev` (`git merge --ff-only origin/dev`).
   `test` and `main` each hold one merge commit that `dev` does not; that is expected.

Commit messages end with the Co-Authored-By line, and PR descriptions with the "Generated with
Claude Code" line, as the session instructions give.

## The promotion-order gate

The CI job **"Promotion order (dev, then test, then main)"** (`devtools promotion-gate`, in
`cmd/devtools/promotion.go`) runs on every push to `test` and `main`. It requires the commit to
already be on the previous branch and CI to have passed there.

The merge commit GitHub creates when a promotion PR is merged exists only on the target branch.
The gate therefore also accepts a two-parent commit whose tree is identical to its second parent
(what a promotion merge is) and judges the second parent instead: it must be on the previous
branch with a passing CI run. A merge that adds anything of its own is refused.

If the gate fails:
- "is not on dev/test yet": the change skipped a stage. Go back and promote it in order.
- "no CI run found" or "CI is still running": wait for the run on the previous branch to finish and
  pass, then re-run the failed job (`gh run rerun <run-id> --failed`).
- "CI did not pass": fix it on `dev` and promote again.
- Never get round it with a direct push, a force-push, or by editing the gate to wave a commit
  through. If branch protection or the permission system blocks an action, stop and ask.

## Branch protection (already configured)

`dev`, `test` and `main` require nine status checks (Frontend lint, Build/vet/test, Browser tests,
govulncheck, Docs updated, Trivy FS, actionlint, CodeQL, staticcheck/gosec) and forbid force-pushes
and deletions. `enforce_admins` is off.

## Waiting for CI

`gh pr checks <n>` lists the checks; `gh run list --branch <b> --limit 3` shows the push runs. Do
not poll in a loop with `sleep`; use a background watch such as
`until ! gh pr checks <n> | grep -q pending; do sleep 10; done`.
