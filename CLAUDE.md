# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is the Honeycomb fork of the GenAI Prices project — a database of LLM inference API pricing.
It has been reduced to the **Go implementation** plus the bundled price data it depends on. The
Python and JavaScript/TypeScript implementations that live in
[`pydantic/genai-prices`](https://github.com/pydantic/genai-prices) are **not** maintained here.

The repository contains:

- **Go Package** (`packages/go/`): the library for calculating costs, published as
  `github.com/honeycombio/genai-prices/packages/go`
- **Price Data** (`prices/`): the compiled price catalog and JSON Schemas the Go package embeds

## Architecture

### Key Directories

- `packages/go/`: the Go package
  - `genaiprices.go`, `engine.go`, `match.go`, `extract.go`, `types.go`: implementation
  - `data.go`: embeds `data.json` via `//go:embed`
  - `data.json`: a copy of `prices/data.json`, embedded at compile time (DO NOT EDIT directly)
- `prices/`: the bundled price data
  - `data.json` / `data_slim.json`: compiled price catalogs (DO NOT EDIT directly)
  - `data.schema.json` / `data_slim.schema.json`: JSON Schemas for the above

## Price Data

The price data is sourced from upstream `pydantic/genai-prices` and is **not** built in this
repository — the build pipeline lives upstream.

- **NEVER** edit `prices/data.json`, `prices/data_slim.json`, or `packages/go/data.json` by hand.
- `packages/go/data.json` must be kept in sync with `prices/data.json` (it is a copy embedded by
  the Go package).
- Upstream releases are detected by a Dependabot watch on
  `upstream-watch/requirements.txt` (see the maintainer note in `README.md`). A PR
  labelled `upstream-release` is the signal to refresh the price data and, if the schema or
  pricing logic changed, update the Go implementation to match.

## Development Commands

All commands operate on the Go package (see the `Makefile`):

```bash
make build   # go build ./...
make test    # go test ./...
make lint    # gofmt check + go vet
make format  # gofmt -w
make install # install pre-commit hooks
```

Or run Go tooling directly from `packages/go/`:

```bash
cd packages/go
go build ./...
go test ./...
go vet ./...
```

## Code Style

- Format Go code with `gofmt` (enforced in CI and via pre-commit).
- Follow existing patterns in the `packages/go/` codebase.
- Generic pre-commit hooks (YAML/TOML checks, codespell, zizmor, whitespace) run repo-wide;
  codespell config is in `.codespellrc`.
