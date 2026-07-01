# Syncing this fork with upstream

`honeycombio/genai-prices` is a **stripped-down** fork of
[`pydantic/genai-prices`](https://github.com/pydantic/genai-prices). We do **not** vendor
upstream's Python/JS packages, price YAMLs, or build pipeline — only this Go package
(`packages/go/`) and the two files it needs from upstream:

- **`packages/go/data.json`** — the full price catalog, embedded via `//go:embed` and decoded
  into the hand-written structs in `types.go`. This is the runtime data.
- **`packages/go/data.schema.json`** — upstream's JSON Schema for that data. Nothing imports it;
  it exists purely as a **change signal** — when it changes, our Go structs probably need to too.

> **Not a rebasable fork.** Because upstream's source is gone from this repo, you cannot
> `git rebase upstream/main` anymore. Syncing means *pulling the two files above*, not replaying
> our patch on top of upstream. This is the deliberate trade for a minimal dependency/vulnerability
> surface (no Python or npm dependency trees).

## 0. Get notified

[`upstream-watch/requirements.txt`](../../upstream-watch/requirements.txt) pins the last upstream
version we synced to. Dependabot checks it daily and opens a PR labelled `upstream-release` when
PyPI has a newer `genai-prices`. The **"Upstream data diff"** workflow
([`upstream-data-diff.yml`](../../.github/workflows/upstream-data-diff.yml) →
[`upstream-data-diff.sh`](./upstream-data-diff.sh)) comments on that PR with whether the schema
changed between our vendored copy and the new upstream version — a schema change means the Go
structs below likely need updating.

## 1. Refresh the two vendored files

Run the sync script with the new upstream version (it fetches from the `v<version>` tag):

```bash
packages/go/sync-upstream-data.sh <new-version>     # e.g. 0.0.67
```

This overwrites `packages/go/data.json` and `packages/go/data.schema.json` from upstream. Do this
on a branch and land it via PR (step 4). Review the diff before committing:

```bash
git diff -- packages/go/data.json packages/go/data.schema.json
```

## 2. Check for schema drift

The Dependabot PR comment (step 0) already told you whether the schema changed. Confirm from the
`git diff` above:

- **No `data.schema.json` diff** — the data format is unchanged; the Go structs are still valid.
  Continue.
- **`data.schema.json` changed** — the format moved. Update `packages/go/types.go`
  (+ `match.go` / `extract.go`) to match.

`data.json` is decoded into hand-written Go structs, and `json.Unmarshal` is lenient: an
upstream-**added** field is silently ignored, and a **renamed** field decodes to a zero value —
either can make the package return wrong prices with no error. A type-level change (a field
changing JSON shape) fails loudly instead: `init()` in `data.go` panics, and the custom
`UnmarshalJSON` in `match.go` rejects unknown match operators. Only silent additive/rename drift
needs the manual check.

> **Engine behavior, not just data shape.** The Go code is a hand-written **port** of upstream's
> pricing engine — and upstream's reference implementation no longer lives in this repo. If an
> upstream release changed pricing *logic* (tiered pricing, match rules, usage extraction), the
> schema diff won't catch it. When in doubt, review upstream's source directly for the version
> you're syncing:
> [`packages/python/genai_prices/`](https://github.com/pydantic/genai-prices/tree/main/packages/python/genai_prices)
> and [`packages/js/src/`](https://github.com/pydantic/genai-prices/tree/main/packages/js/src)
> (`engine.ts`, `extractUsage.ts`).

## 3. Verify

```bash
cd packages/go && go vet ./... && go test ./...
```

`go test` exercises the embedded `data.json` end to end (it must parse and price a known model).

## 4. Open a PR for review

```bash
git switch -c sync/genai-prices-<new-version>
git add packages/go/data.json packages/go/data.schema.json packages/go/*.go
git commit -m "sync(go): pull upstream <new-version> data (+ struct updates if any)"
git push -u origin sync/genai-prices-<new-version>
gh pr create --base main --title "sync: pull upstream <new-version>" --body "..."
```

Scope review to `packages/go/`. Wait for CI (`test-go`) and approval, then merge normally. Cut a
fresh release tag if shipping a new Go module version.

## 5. Bump the tracked version

Update the pin so Dependabot stops re-flagging the version you just synced:

```bash
$EDITOR upstream-watch/requirements.txt   # genai-prices==<new-version>
```

Include this in the same PR as the sync, or a follow-up — either is fine.
