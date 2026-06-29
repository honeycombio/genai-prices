#!/usr/bin/env bash
# Diff prices/data.schema.json and prices/data.json between two upstream
# (pydantic/genai-prices) release versions and render a markdown report.
#
# Used by .github/workflows/upstream-data-diff.yml on Dependabot upstream-release
# PRs, but runnable standalone:
#
#   scripts/upstream-data-diff.sh <old-version> <new-version> [--pr <number>]
#
# Env:
#   UPSTREAM_REPO  upstream repo (default pydantic/genai-prices)
#   OUT            markdown output path (default: a tempfile; printed when no --pr)
#   GH_TOKEN       used by gh; required to fetch files and to post a comment
#
# With --pr, posts/updates a single sticky comment (matched by a hidden marker)
# on that PR in the *current* repo. Without it, the report is printed to stdout.
set -euo pipefail

MARKER='<!-- upstream-data-diff -->'
UPSTREAM_REPO="${UPSTREAM_REPO:-pydantic/genai-prices}"
LIST_LIMIT=20

usage() {
  echo "usage: $0 <old-version> <new-version> [--pr <number>]" >&2
  exit 2
}

OLD=""
NEW=""
PR=""
while [ $# -gt 0 ]; do
  case "$1" in
    --pr)
      PR="${2:-}"
      shift 2
      ;;
    -h | --help) usage ;;
    -*) echo "unknown flag: $1" >&2; usage ;;
    *)
      if [ -z "$OLD" ]; then
        OLD="$1"
      elif [ -z "$NEW" ]; then
        NEW="$1"
      else
        echo "unexpected argument: $1" >&2
        usage
      fi
      shift
      ;;
  esac
done
if [ -z "$OLD" ] || [ -z "$NEW" ]; then usage; fi

WORK="$(mktemp -d)"
OUT="${OUT:-$WORK/report.md}"

# Post (or sticky-update) the report on PR $PR. Best-effort: never aborts.
post_comment() {
  local body="$1" existing_id
  existing_id="$(gh api "repos/{owner}/{repo}/issues/$PR/comments" --paginate \
    --jq "[.[] | select(.body | contains(\"$MARKER\")) | .id] | first // empty" 2>/dev/null || true)"
  if [ -n "$existing_id" ]; then
    gh api "repos/{owner}/{repo}/issues/comments/$existing_id" -X PATCH -F body=@"$body" >/dev/null \
      && echo "Updated comment $existing_id on PR #$PR." \
      || echo "warning: failed to update comment on PR #$PR" >&2
  else
    gh api "repos/{owner}/{repo}/issues/$PR/comments" -F body=@"$body" >/dev/null \
      && echo "Created comment on PR #$PR." \
      || echo "warning: failed to create comment on PR #$PR" >&2
  fi
}

# Always deliver *something*: on a non-zero exit the report is partial (or never
# started), so append a failure note, then post/print whatever we have.
on_exit() {
  local rc=$?
  if [ "$rc" -ne 0 ]; then
    if [ ! -s "$OUT" ]; then
      { echo "$MARKER"; echo "## Upstream data diff"; echo; } >"$OUT" 2>/dev/null || true
    fi
    {
      echo
      echo "> ❌ The diff script exited with status $rc before completing — the report above may be partial."
      echo "> Re-run \`scripts/upstream-data-diff.sh $OLD $NEW\` or check the workflow logs."
    } >>"$OUT" 2>/dev/null || true
  fi
  if [ -n "$PR" ] && [ -s "$OUT" ]; then
    post_comment "$OUT"
  elif [ -z "$PR" ] && [ -s "$OUT" ]; then
    cat "$OUT"
  fi
  rm -rf "$WORK"
}
trap on_exit EXIT

# Fetch prices/<file> at tag v<version> from the upstream repo into <dest>.
# Returns 0 on success, 1 if the tag/file is missing (e.g. tag not pushed yet).
fetch_file() {
  local version="$1" file="$2" dest="$3" tmp
  tmp="$(mktemp)"
  if gh api "repos/$UPSTREAM_REPO/contents/prices/$file?ref=v$version" \
    -H "Accept: application/vnd.github.raw" >"$tmp" 2>/dev/null; then
    mv "$tmp" "$dest"
    return 0
  fi
  rm -f "$tmp"
  : >"$dest" # leave dest empty so [ -s ] guards treat it as missing
  return 1
}

# Emit "key\t<compact-json>" for every model, sorted by key (provider_id/model_id).
model_kv() {
  jq -r '.[] | .id as $p | .models[]? | [($p + "/" + (.id // .name)), (. | @json)] | @tsv' "$1" | sort
}

# Render up to LIST_LIMIT lines from stdin as a bulleted markdown list, with an
# overflow note. Prints nothing if input is empty.
render_list() {
  local f="$WORK/list.$$"
  cat >"$f"
  local n
  n=$(wc -l <"$f" | tr -d ' ')
  [ "$n" -eq 0 ] && { rm -f "$f"; return; }
  sed "s/^/- \`/; s/$/\`/" "$f" | head -n "$LIST_LIMIT"
  if [ "$n" -gt "$LIST_LIMIT" ]; then
    echo "- … $((n - LIST_LIMIT)) more"
  fi
  rm -f "$f"
}

# ---- fetch all four files -------------------------------------------------
schema_old="$WORK/schema.old"; schema_new="$WORK/schema.new"
data_old="$WORK/data.old"; data_new="$WORK/data.new"

missing=""
fetch_file "$OLD" data.schema.json "$schema_old" || missing="$missing v$OLD"
fetch_file "$NEW" data.schema.json "$schema_new" || missing="$missing v$NEW"
fetch_file "$OLD" data.json "$data_old" || true
fetch_file "$NEW" data.json "$data_new" || true

{
  echo "$MARKER"
  echo "## Upstream data diff: \`$OLD\` → \`$NEW\`"
  echo
  echo "Comparing [\`$UPSTREAM_REPO\`](https://github.com/$UPSTREAM_REPO) tags \`v$OLD\` and \`v$NEW\`."
  echo
} >"$OUT"

if [ -n "$missing" ]; then
  {
    echo "> ⏳ Could not fetch upstream tag(s):$missing — the git tag may not be pushed yet."
    echo "> Re-run once the upstream release is tagged."
    echo
  } >>"$OUT"
fi

# ---- data.schema.json -----------------------------------------------------
{
  echo "### \`prices/data.schema.json\`"
  echo
} >>"$OUT"

if [ -s "$schema_old" ] && [ -s "$schema_new" ]; then
  if diff -q "$schema_old" "$schema_new" >/dev/null; then
    echo "✅ **No schema change.**" >>"$OUT"
  else
    {
      echo "⚠️ **Schema changed — the Go implementation in \`packages/go/\` likely needs updating.**"
      echo
      echo '```diff'
      diff -u --label "v$OLD/prices/data.schema.json" --label "v$NEW/prices/data.schema.json" \
        "$schema_old" "$schema_new" || true
      echo '```'
    } >>"$OUT"
  fi
else
  echo "_Schema unavailable for one or both versions (see note above)._" >>"$OUT"
fi

echo >>"$OUT"

# ---- data.json ------------------------------------------------------------
{
  echo "### \`prices/data.json\`"
  echo
} >>"$OUT"

if [ -s "$data_old" ] && [ -s "$data_new" ]; then
  # Pretty-print (sorted keys) for a meaningful diff; the source is minified.
  jq -S . "$data_old" >"$WORK/data.old.pretty"
  jq -S . "$data_new" >"$WORK/data.new.pretty"

  if diff -q "$WORK/data.old.pretty" "$WORK/data.new.pretty" >/dev/null; then
    echo "✅ **No data change.**" >>"$OUT"
  else
    # Provider-level changes.
    jq -r '.[].id' "$data_old" | sort -u >"$WORK/prov.old"
    jq -r '.[].id' "$data_new" | sort -u >"$WORK/prov.new"
    prov_added="$(comm -13 "$WORK/prov.old" "$WORK/prov.new")"
    prov_removed="$(comm -23 "$WORK/prov.old" "$WORK/prov.new")"

    # Model-level changes (key = provider_id/model_id).
    model_kv "$data_old" >"$WORK/kv.old"
    model_kv "$data_new" >"$WORK/kv.new"
    cut -f1 "$WORK/kv.old" >"$WORK/k.old"
    cut -f1 "$WORK/kv.new" >"$WORK/k.new"
    model_added="$(comm -13 "$WORK/k.old" "$WORK/k.new")"
    model_removed="$(comm -23 "$WORK/k.old" "$WORK/k.new")"
    # Models present in both versions whose JSON differs.
    model_changed="$(join -t"$(printf '\t')" "$WORK/kv.old" "$WORK/kv.new" \
      | awk -F'\t' '$2 != $3 { print $1 }')"

    count() { if [ -z "$1" ]; then echo 0; else printf '%s\n' "$1" | grep -c .; fi; }

    {
      echo "Models: **+$(count "$model_added")** added · **−$(count "$model_removed")** removed · **~$(count "$model_changed")** changed."
      pa=$(count "$prov_added"); pr=$(count "$prov_removed")
      if [ "$pa" -ne 0 ] || [ "$pr" -ne 0 ]; then
        echo
        echo "Providers: **+$pa** added · **−$pr** removed."
      fi
      echo
    } >>"$OUT"

    emit_section() {
      local title="$1" data="$2"
      [ -z "$data" ] && return
      {
        echo "<details><summary>$title ($(count "$data"))</summary>"
        echo
        printf '%s\n' "$data" | render_list
        echo
        echo "</details>"
      } >>"$OUT"
    }
    emit_section "Providers added" "$prov_added"
    emit_section "Providers removed" "$prov_removed"
    emit_section "Models added" "$model_added"
    emit_section "Models removed" "$model_removed"
    emit_section "Models changed" "$model_changed"

    {
      echo
      echo "<details><summary>Full pretty-printed diff</summary>"
      echo
      echo '```diff'
      diff -u --label "v$OLD/prices/data.json" --label "v$NEW/prices/data.json" \
        "$WORK/data.old.pretty" "$WORK/data.new.pretty" || true
      echo '```'
      echo
      echo "</details>"
    } >>"$OUT"
  fi
else
  echo "_data.json unavailable for one or both versions (see note above)._" >>"$OUT"
fi

# Posting / printing happens in the on_exit trap, so a partial report from an
# early failure is still delivered to the PR.
