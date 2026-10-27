#!/usr/bin/env bash
# Freeze the working docs (internal/docs/content/next) as a release snapshot.
#
#   .github/scripts/docs-snapshot.sh 1.42
#
# Entries and "since" values labelled "next" belong to the release being cut,
# so they are relabelled to the new version in BOTH the working copy and the
# snapshot (otherwise the next release would claim to have introduced them).
# Run from the repo root, commit the result, and read the snapshot in the
# running app before merging. See .claude/skills/docs/SKILL.md.
set -euo pipefail

ver="${1:-}"
if ! [[ "$ver" =~ ^1\.[0-9]+$ ]]; then
  echo "usage: $0 1.<number>   (e.g. 1.42)" >&2
  exit 2
fi
root="internal/docs/content"
if [ -e "$root/$ver" ]; then
  echo "$root/$ver already exists: released snapshots are never overwritten" >&2
  exit 1
fi
if [ ! -d "$root/next" ]; then
  echo "$root/next not found (run from the repository root)" >&2
  exit 1
fi

v="${ver//./\\.}"
for f in "$root"/next/*.md; do
  # Only front matter lines have this exact shape; body text is untouched.
  sed -i -E "s/^  - next: /  - $v: /; s/^since: next\$/since: $v/" "$f"
done
cp -r "$root/next" "$root/$ver"

go test ./internal/docs -count=1
echo "Snapshot $ver created. Review it in the app (Help > Version), then commit."
