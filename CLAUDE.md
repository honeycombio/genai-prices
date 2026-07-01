# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is a **stripped-down fork** of [`pydantic/genai-prices`](https://github.com/pydantic/genai-prices).
Upstream's Python package, JavaScript/TypeScript package, price-source YAMLs, and build pipeline
have all been removed to minimise the dependency/vulnerability surface. The entire delta this fork
maintains is:

- **`packages/go/`** — a Go package that calculates LLM inference prices (a hand-written port of
  upstream's Python/JS engine).
- Two vendored data files it depends on, tracked from upstream.

## Architecture

### Key files (all under `packages/go/`)

- `data.json` — full price catalog vendored from upstream, embedded at compile time via
  `//go:embed` (see `data.go`). **DO NOT edit directly** — it is refreshed from upstream.
- `data.schema.json` — upstream's JSON Schema for the catalog. Not imported by any code; kept
  purely as a **change signal** for when the Go structs need updating.
- `types.go` — hand-written structs the catalog decodes into.
- `engine.go` / `match.go` / `extract.go` — the pricing engine, provider/model matching, and
  usage extraction (the port of upstream logic).
- `genaiprices.go` — public API (`CalcPrice`, `FindProvider`, `ExtractUsage`).
- `genaiprices_test.go` — smoke test exercising the embedded data end to end.
- `sync-upstream-data.sh` — refreshes the two vendored files from an upstream release.
- `upstream-data-diff.sh` — reports whether upstream's schema changed vs. our vendored copy.
- `SYNCING.md` — the upstream-sync runbook.

## Development Commands

Everything is standard Go, run from `packages/go/`:

```bash
cd packages/go
go vet ./...       # static checks
go test ./...      # run tests (parses + prices against the embedded data.json)
go build ./...     # build
```

There is no Makefile, no `uv`, and no Node tooling in this fork.

## Syncing with upstream

- `upstream-watch/requirements.txt` pins the last upstream version synced. Dependabot opens an
  `upstream-release` PR when `pydantic/genai-prices` publishes a newer version; the "Upstream data
  diff" workflow (`.github/workflows/upstream-data-diff.yml`) comments whether the schema changed.
- To sync: run `packages/go/sync-upstream-data.sh <version>`, review the diff, update the Go structs
  if `data.schema.json` changed, then `go test ./...`. Full runbook: `packages/go/SYNCING.md`.

## Important Notes

- **DO NOT edit `packages/go/data.json` by hand** — it is vendored from upstream. Change it only via
  `sync-upstream-data.sh`.
- The Go code is a **port**; upstream's reference implementation is no longer in this repo. When
  upstream changes pricing *logic* (not just data shape), review upstream's source on GitHub and
  port the change into `packages/go/`. See the "Engine behavior" note in `SYNCING.md`.
- The module path is `github.com/honeycombio/genai-prices/packages/go`.
