# genai-prices (Go, honeycombio fork)

Calculate prices for calling LLM inference APIs, from Go.

This is a **stripped-down fork** of
[`pydantic/genai-prices`](https://github.com/pydantic/genai-prices), reduced to the pieces
Honeycomb needs for its infrastructure:

- **`packages/go/`** — a Go package that calculates LLM prices (a port of upstream's Python/JS
  engine), plus the two data files it embeds.

Upstream's Python and JavaScript/TypeScript packages, the price-source YAMLs, and the build
pipeline have all been removed to minimise the dependency and vulnerability surface we carry.
We track upstream's data by vendoring just two files (see [Syncing](#syncing-with-upstream)):

- `packages/go/data.json` — the full price catalog, embedded at compile time via `//go:embed`.
- `packages/go/data.schema.json` — upstream's JSON Schema for that data, kept as a change signal.

## Usage

See the [Go package README](packages/go/README.md) for installation and examples.

```bash
go get github.com/honeycombio/genai-prices/packages/go
```

## Syncing with upstream

Dependabot watches upstream releases (via [`upstream-watch/requirements.txt`](upstream-watch/requirements.txt))
and opens an `upstream-release` PR when `pydantic/genai-prices` publishes a new version. A CI
workflow then reports whether the price-data schema changed.

> **Note:** the sync tooling ([`packages/go/SYNCING.md`](packages/go/SYNCING.md),
> `upstream-data-diff.sh`) still assumes the old full-fork layout and is being reworked in a
> separate PR after this strip.

## ⚠️ Warning: these prices will not be 100% accurate

This project is a best effort from Pydantic and the community to provide an indicative estimate of
the price you might pay for calling an LLM. Providers do not publish exact, machine-readable pricing,
so the data cannot be guaranteed correct. If you get a bill you weren't expecting, don't blame us.

## License & attribution

MIT — © Pydantic Services Inc. See [LICENSE](LICENSE). This fork redistributes upstream's price data
unchanged; all price-data credit belongs to the upstream project and its sources:
[Helicone](https://github.com/Helicone/helicone/tree/main/packages/cost),
[OpenRouter](https://openrouter.ai/docs/api/api-reference/models/get-models),
[LiteLLM](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json), and
Simon Willison's [llm-prices](https://github.com/simonw/llm-prices).
