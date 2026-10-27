#!/usr/bin/env bash
# Builds the server, boots it against a throwaway DB, fetches the public
# (unauthenticated) pages and validates the rendered HTML. The templates
# themselves are Go templates, which HTML linters can't parse, so the
# rendered output is what gets checked. Run from the repo root; needs
# node/npx and go.
set -euo pipefail

tmp="$(mktemp -d)"
trap 'kill "${srv_pid:-}" 2>/dev/null || true; rm -rf "$tmp"' EXIT

go build -o "$tmp/server" ./cmd/server
SESSION_KEY="html-validate-session-key-0123456789abcdef" COOKIE_SECURE=false \
  SKIP_EMAIL_VERIFICATION=true PORT=18123 DB_PATH="$tmp/t.db" "$tmp/server" >"$tmp/server.log" 2>&1 &
srv_pid=$!

for _ in $(seq 1 20); do
  curl -sf http://localhost:18123/health >/dev/null && break
  sleep 0.5
done

mkdir "$tmp/pages"
for p in login claim; do
  curl -sf "http://localhost:18123/$p" >"$tmp/pages/$p.html"
done

# The docs sit behind login: claim a squadron and sign in (dev-mode skips the
# confirmation email) so the rendered docs pages are validated too.
jar="$tmp/jar"
csrf() { curl -sf -b "$jar" -c "$jar" "http://localhost:18123/$1" | sed -n 's/.*name="csrf" value="\([0-9a-f]*\)".*/\1/p' | head -1; }
curl -sf -b "$jar" -c "$jar" -o /dev/null -d "csrf=$(csrf claim)" \
  --data-urlencode "email=oc.1@rafac.mod.gov.uk" --data-urlencode "password=a long enough password" http://localhost:18123/claim
curl -sf -b "$jar" -c "$jar" -o /dev/null -d "csrf=$(csrf login)" \
  --data-urlencode "email=oc.1@rafac.mod.gov.uk" --data-urlencode "password=a long enough password" http://localhost:18123/login
for slug in $(sed -n 's/^.*](\([a-z0-9-]*\)).*$/\1/p' internal/docs/content/next/index.md | sort -u) index; do
  path="docs/next/$slug"; [ "$slug" = index ] && path="docs/next/"
  curl -sf -b "$jar" "http://localhost:18123/$path" >"$tmp/pages/docs-$slug.html"
done

npx --yes html-validate@9.5.5 -c .htmlvalidate.json "$tmp"/pages/*.html
