#!/usr/bin/env bash
# Refreshes the two vendored files this fork tracks from upstream
# (pydantic/genai-prices) into packages/go/:
#
#   packages/go/data.json         <- upstream prices/data.json         (embedded via //go:embed)
#   packages/go/data.schema.json  <- upstream prices/data.schema.json  (schema-drift signal)
#
# This replaces upstream's Python `make package-data` step, which no longer
# exists in this stripped-down fork. Run it as part of an upstream sync (see
# SYNCING.md), review the diff, update the Go structs if the schema changed,
# then `go test ./...`.
#
# Usage: sync-upstream-data.sh <version>          # fetches from tag v<version>
#
# Env:
#   UPSTREAM_REPO  upstream repo (default pydantic/genai-prices)
#   REF            git ref to fetch from (default "v<version>"); set to "main"
#                  to preview unreleased data before a tag is pushed
#   GH_TOKEN       used by gh (optional locally; raises rate limits)
set -euo pipefail

UPSTREAM_REPO="${UPSTREAM_REPO:-pydantic/genai-prices}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() { printf 'usage: %s <version>\n' "$0" >&2; exit 2; }
[[ $# -eq 1 ]] || usage
version="${1#v}"                 # tolerate a leading "v"
ref="${REF:-v$version}"

fetch() {
  # fetch <upstream-path> <dest-path>
  local src="$1" dest="$2" tmp
  tmp=$(mktemp)
  if ! gh api "repos/$UPSTREAM_REPO/contents/$src?ref=$ref" \
       -H "Accept: application/vnd.github.raw" > "$tmp" 2>/dev/null; then
    printf 'error: could not fetch %s@%s from %s (is the tag pushed?)\n' \
      "$src" "$ref" "$UPSTREAM_REPO" >&2
    rm -f "$tmp"; exit 1
  fi
  [[ -s "$tmp" ]] || { printf 'error: fetched empty %s\n' "$src" >&2; rm -f "$tmp"; exit 1; }
  mv "$tmp" "$dest"
  printf 'updated %s\n' "$dest"
}

fetch "prices/data.json"        "$SCRIPT_DIR/data.json"
fetch "prices/data.schema.json" "$SCRIPT_DIR/data.schema.json"

printf '\nDone. Next: review the diff, update Go structs if the schema changed, then:\n'
printf '  cd packages/go && go vet ./... && go test ./...\n'
