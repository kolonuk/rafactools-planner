#!/usr/bin/env bash
# Pull-request gate: a change to user-visible code must come with a change to
# the docs (internal/docs/content/), unless the PR description says why not.
#
# Env: BASE, HEAD (commit SHAs), PR_BODY (the PR description).
# The body arrives via the environment, never interpolated into the script.
set -euo pipefail

: "${BASE:?}" "${HEAD:?}"
changed="$(git diff --name-only "$BASE...$HEAD")"

visible="$(printf '%s\n' "$changed" \
  | grep -E '^(internal/(web|config|auth|mail|pdfgen)/|cmd/server/|Dockerfile$|docker-compose\.yml$|\.github/workflows/)' \
  | grep -vE '_test\.go$' || true)"

if [ -z "$visible" ]; then
  echo "No user-visible files changed; docs not required."
  exit 0
fi

if printf '%s\n' "$changed" | grep -q '^internal/docs/content/'; then
  echo "User-visible files changed and the docs were updated."
  exit 0
fi

if printf '%s\n' "${PR_BODY:-}" | grep -qE '^No docs impact: .{15,}'; then
  echo "No docs impact declared in the PR description:"
  printf '%s\n' "$PR_BODY" | grep -E '^No docs impact: '
  exit 0
fi

echo "::error::User-visible files changed but internal/docs/content/ did not."
echo "Files:"
printf '  %s\n' $visible
echo
echo "Update the matching pages in internal/docs/content/next/ (see .claude/skills/docs/SKILL.md),"
echo "or add a line to the PR description:  No docs impact: <specific reason>"
exit 1
