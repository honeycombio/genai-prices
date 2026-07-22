# Syncing price data from upstream

`honeycombio/genai-prices` is a standalone Go library. It is **not** maintained as a fork
of [`pydantic/genai-prices`](https://github.com/pydantic/genai-prices) — the Go
implementation lives here, and the only things we take from upstream are the compiled
price data files and their schemas:

- `prices/data.json` / `prices/data.schema.json` — `prices/data.json` is embedded directly via
  `//go:embed` (see `data.go`)
- `prices/data_slim.json` / `prices/data_slim.schema.json` — not used by the Go package

We never hand-edit these; the build pipeline that produces them lives upstream.

## 0. Get notified

[`upstream-watch/requirements.txt`](upstream-watch/requirements.txt) pins the last
upstream version we synced to. Dependabot checks it daily and opens a PR labelled
`upstream-release` when PyPI has a newer `genai-prices`. The "Upstream data diff" workflow
comments on that PR with whether `prices/data.schema.json` changed — a schema change means
the Go structs below likely need updating.

## 1. Pull the refreshed data

Fetch the data files from the upstream release tag (same mechanism as
[`upstream-data-diff.sh`](upstream-data-diff.sh)) on a branch:

```bash
NEW=<new-version>   # e.g. 0.0.67
git switch -c sync-upstream-$NEW main

for f in data.json data.schema.json data_slim.json data_slim.schema.json; do
  gh api "repos/pydantic/genai-prices/contents/prices/$f?ref=v$NEW" \
    -H "Accept: application/vnd.github.raw" > "prices/$f"
done

git add prices/
git commit -m "sync: refresh price data from upstream v$NEW"
```

## 2. Check for schema drift

The Dependabot PR comment (step 0) already told you whether `prices/data.schema.json`
changed. Confirm locally:

```bash
git diff main -- prices/data.schema.json
```

- **No diff** — the data format is unchanged; the Go structs are still valid. Continue.
- **Diff** — the format changed. It names exactly which fields/types moved. Update
  `types.go` (+ `match.go` / `extract.go`) to match, and commit that as its
  own change on top (e.g. `fix(go): match upstream schema change in <field>`).

`prices/data.json` is embedded via `//go:embed` and decoded into hand-written Go
structs. `json.Unmarshal` is lenient: an upstream-**added** field is silently ignored, and
a **renamed** field decodes to a zero value — either can make the package return wrong
prices with no error. A type-level change (a field changing JSON shape) fails loudly
instead: `init()` in `data.go` panics, and the custom `UnmarshalJSON` in `match.go`
rejects unknown match operators. Only silent additive/rename drift needs the manual check
above.

## 3. Verify

```bash
make lint test   # gofmt + go vet + go test
```

## 4. Bump the tracked version and open a PR

```bash
$EDITOR upstream-watch/requirements.txt   # genai-prices==<new-version>
git commit -am "sync: bump genai-prices to $NEW"
git push -u origin sync-upstream-$NEW
gh pr create --base main --title "sync: pull upstream v$NEW price data" --body "..."
```

Merge normally once approved. Bumping the pin stops Dependabot from re-flagging the
version you just synced, and closes its `upstream-release` PR.
