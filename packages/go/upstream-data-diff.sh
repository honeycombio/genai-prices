#!/usr/bin/env bash
# Diffs the price-data schema between our vendored copy (packages/go/data.schema.json)
# and an upstream (pydantic/genai-prices) release version, then renders a markdown report.
#
# Upstream keeps the schema at prices/data.schema.json; we vendor it into packages/go/.
#
# Usage: upstream-data-diff.sh <new-version> [<pr-number>]
#
# Env:
#   UPSTREAM_REPO  upstream repo (default pydantic/genai-prices)
#   GH_TOKEN       used by gh; required to fetch files and post comments
set -uo pipefail

UPSTREAM_REPO="${UPSTREAM_REPO:-pydantic/genai-prices}"
SCHEMA="data.schema.json"
LOCAL_SCHEMA="packages/go/$SCHEMA"       # our vendored copy
UPSTREAM_SCHEMA="prices/$SCHEMA"         # where it lives in upstream's tree

usage() { printf 'usage: %s <new-version> [<pr-number>]\n' "$0" >&2; exit 2; }

[[ $# -ge 1 ]] || usage
new_v="$1"; pr="${2:-}"

deliver() {
  local body="$1"
  if [[ -z "$pr" ]]; then
    printf '%s' "$body"; return
  fi
  local tmp; tmp=$(mktemp)
  printf '%s' "$body" > "$tmp"
  gh api -X POST "repos/{owner}/{repo}/issues/$pr/comments" -F "body=@$tmp" >/dev/null \
    && printf 'Created comment on PR #%s.\n' "$pr" \
    || printf 'warning: failed to create comment\n' >&2
  rm -f "$tmp"
}

build_err=""
report="Comparing our vendored \`${LOCAL_SCHEMA}\` with upstream \`${UPSTREAM_SCHEMA}\` at \`v${new_v}\`."$'\n\n'
report+="### \`${SCHEMA}\`"$'\n\n'

schema_old=$(cat "$LOCAL_SCHEMA" 2>/dev/null); old_ok=$?
schema_new=$(gh api "repos/$UPSTREAM_REPO/contents/$UPSTREAM_SCHEMA?ref=v$new_v" \
  -H "Accept: application/vnd.github.raw" 2>/dev/null); new_ok=$?

if [[ $old_ok -ne 0 || $new_ok -ne 0 ]]; then
  report+="> ⏳ Could not fetch the schema (the git tag may not be "
  report+="pushed yet). Re-run once the upstream release is tagged."$'\n'
else
  diff -q <(printf '%s' "$schema_old") <(printf '%s' "$schema_new") >/dev/null
  diff_exit=$?
  if [[ $diff_exit -eq 0 ]]; then
    report+="✅ **No schema change.**"$'\n'
  elif [[ $diff_exit -eq 1 ]]; then
    report+="⚠️ **Schema changed — the Go implementation in \`packages/go/\` likely needs updating.**"$'\n'
  else
    build_err="diff exited $diff_exit"
  fi
fi

report+=$'\n'

if [[ -n "$build_err" ]]; then
  report+="> ❌ The diff tool failed before completing — the report above may be partial."$'\n'
  report+="> Error: ${build_err}"$'\n'
fi

deliver "$report"
[[ -z "$build_err" ]]
