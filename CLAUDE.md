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
- `upstream-data-diff.sh` — reports whether upstream's schema changed (part of the sync tooling
  being reworked separately — see below).
- `SYNCING.md` — the upstream-sync runbook (out of date after this strip — see below).

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

Dependabot opens an `upstream-release` PR (via `upstream-watch/requirements.txt`) when
`pydantic/genai-prices` publishes a newer version, and the "Upstream data diff" workflow
(`.github/workflows/upstream-data-diff.yml`) comments whether the schema changed.

> **Syncing is being reworked in a separate PR.** This strip removed the Python build pipeline
> (`make package-data`) that `SYNCING.md` and `upstream-data-diff.sh` were written around, so they
> reference paths (`prices/…`) that no longer exist. Don't rely on that tooling until the follow-up
> lands; for now, refresh `packages/go/data.json` and `data.schema.json` from upstream by hand.

## Important Notes

- **DO NOT edit `packages/go/data.json` by hand** — it is vendored from upstream. Change it only via
  `sync-upstream-data.sh`.
- The Go code is a **port**; upstream's reference implementation is no longer in this repo. When
  upstream changes pricing *logic* (not just data shape), review upstream's source on GitHub and
  port the change into `packages/go/`. See the "Engine behavior" note in `SYNCING.md`.
- The module path is `github.com/honeycombio/genai-prices/packages/go`.
