# Syncing this fork with upstream

`honeycombio/genai-prices` is a fork of [`pydantic/genai-prices`](https://github.com/pydantic/genai-prices).
Our **entire delta is this Go package** (`packages/go/`) plus a few small hooks that keep
its embedded data current:

- `packages/go/*` — net-new, never conflicts.
- `prices/src/prices/package_data.py` (`package_go_data`) — copies `prices/data.json`
  into `packages/go/data.json` during `make package-data`.
- `.github/workflows/ci.yml` (`test-go` job), root `README.md`, `.prettierignore` — small
  additions.

Everything else — all price YAMLs, `prices/data.json`, the Python/JS packages — comes
straight from upstream and we never hand-edit it.

## 0. Get notified

[`upstream-watch/requirements.txt`](../../upstream-watch/requirements.txt) pins the last
upstream version we synced to. Dependabot checks it daily and opens a PR labelled
`upstream-release` when PyPI has a newer `genai-prices`. The "Upstream data diff" workflow
comments on that PR with whether `prices/data.schema.json` changed — a schema change means
the Go structs below likely need updating.

## 1. Rebase our change onto the latest upstream

We keep the Go layer as a **clean patch replayed on top of `upstream/main`**, so history
stays linear. Do this on a branch — land it via PR review (step 5), don't push straight to
`main`.

```bash
# one-time
git remote add upstream git@github.com:pydantic/genai-prices.git

# start from a clean tree — commit or stash local work
git stash

git fetch upstream
git switch -c yingrong/sync-upstream main
git rebase upstream/main
#    Expected conflicts ONLY in: .github/workflows/ci.yml, README.md,
#    prices/src/prices/package_data.py — keep BOTH sides (our additions + upstream).
#    Price YAMLs and prices/data.json apply clean. Resolve -> git add -> git rebase --continue.
```

## 2. Rebuild and commit the refreshed data separately

```bash
make build
```

If `packages/go/data.json` changed, commit that on its own, after the rebase — don't fold
it into the replayed upstream commits:

```bash
git add packages/go/data.json
git commit -m "sync(go): refresh embedded data.json from upstream <new-version>"
```

## 3. Check for schema drift

The Dependabot PR comment (step 0) already told you whether `prices/data.schema.json`
changed. Confirm locally:

```bash
git diff upstream/main -- prices/data.schema.json
```

- **No diff** — the data format is unchanged; the Go structs are still valid. Continue.
- **Diff** — the format changed. It names exactly which fields/types moved. Update
  `packages/go/types.go` (+ `match.go` / `extract.go`) to match, and commit that as its
  own change on top (e.g. `fix(go): match upstream schema change in <field>`).

`packages/go/data.json` is embedded via `//go:embed` and decoded into hand-written Go
structs. `json.Unmarshal` is lenient: an upstream-**added** field is silently ignored, and
a **renamed** field decodes to a zero value — either can make the package return wrong
prices with no error. A type-level change (a field changing JSON shape) fails loudly
instead: `init()` in `data.go` panics, and the custom `UnmarshalJSON` in `match.go`
rejects unknown match operators. Only silent additive/rename drift needs the manual check
above.

## 4. Verify

```bash
cd packages/go && go vet ./... && go test ./...
cd ../.. && make test   # python suite
```

## 5. Open a PR for review

```bash
git push -u origin yingrong/sync-upstream
gh pr create --base main --title "sync: pull upstream <new-version>" --body "..."
```

The diff includes all of upstream's changes plus ours — scope review to `packages/go/`
(the rest is just upstream's own commits replayed unchanged). Wait for CI (incl.
`test-go`) and approval.

**Don't merge via GitHub's UI** — "Rebase and merge" (or squash/merge commit) re-SHAs the
commits, so local `main` won't byte-match what was reviewed and the linear-history
property breaks. Once approved, land it manually instead:

```bash
git switch main
git rebase yingrong/sync-upstream   # main == upstream/main + Go-layer commits on top
git push --force-with-lease origin main
```

GitHub detects the commits landing on `main` and marks the PR merged automatically. Cut a
fresh release tag here if shipping a new Go module version.

## 6. Bump the tracked version

```bash
git switch -c yingrong/bump-genai-prices main
$EDITOR upstream-watch/requirements.txt   # genai-prices==<new-version>
git commit -am "sync: bump genai-prices to <new-version>"
git push -u origin yingrong/bump-genai-prices
gh pr create --base main --title "sync: bump genai-prices to <new-version>" --body "..."
```

Merge normally once approved (this one has no history constraints — squash/merge commit
is fine). Stops Dependabot from re-flagging the version you just synced.
