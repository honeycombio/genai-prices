<!-- fork-note:start -->

> **Maintainers — upstream sync via Dependabot.** A Dependabot PR labelled
> `upstream-release` against `upstream-watch/requirements.txt` signals a new
> [`pydantic/genai-prices`](https://github.com/pydantic/genai-prices) release. When handling
> it, refresh the bundled price data (`prices/data.json`, `prices/data_slim.json` and their
> schemas), checking whether `prices/data.schema.json` changed or upstream shipped bug fixes —
> if so, the Go implementation must be updated to match before merging.

<!-- fork-note:end -->

<div align="center">
  <h1>GenAI Prices (Go)</h1>
</div>
<div align="center">
  Calculate prices for calling LLM inference APIs.
</div>
<br/>

This is the [Honeycomb](https://github.com/honeycombio) fork of
[`pydantic/genai-prices`](https://github.com/pydantic/genai-prices), reduced to the **Go
implementation** plus the bundled price data it depends on. The Python and JavaScript/TypeScript
packages that live upstream are not maintained here.

## Features

- Advanced logic for matching on model and provider IDs to maximise the chance of using the correct model
- Support for historic prices and price changes, e.g. the prices for o3 before and after its price changed
- Support for variable daily prices, e.g. deepseek off-peak pricing
- Tiered pricing support for Gemini models where you pay a separate price for very large contexts

## Usage

```bash
go get github.com/honeycombio/genai-prices
```

```go
import genaiprices "github.com/honeycombio/genai-prices"

usage := genaiprices.Usage{InputTokens: 1000, OutputTokens: 100}

calc, err := genaiprices.CalcPrice(usage, "gpt-4o-mini",
    genaiprices.WithProviderID("openai"))
if err != nil {
    log.Fatal(err)
}

fmt.Printf("$%.6f (input $%.6f, output $%.6f) — %s / %s\n",
    calc.TotalPrice, calc.InputPrice, calc.OutputPrice,
    calc.Provider.Name, calc.Model.Name)
```

Provide `WithProviderID` (or `WithProviderAPIURL`) when you know the provider for the most
reliable matching. See [pkg.go.dev](https://pkg.go.dev/github.com/honeycombio/genai-prices) for
the full API — custom/unpublished providers, extracting usage from a raw API response, and
provenance constants (`Name`, `DataSource`, `Version`) for stamping telemetry.

Notes:

- Prices use `float64`, matching the upstream [`pydantic/genai-prices`](https://github.com/pydantic/genai-prices) JavaScript implementation.
- Tiered pricing is threshold-based (cliff): crossing a tier applies that rate to
  all tokens of that bucket.
- `prices/data.json` is generated — DO NOT edit it directly.

## Price data

The bundled price catalog is kept in this repository so the Go package can embed it at compile
time (`//go:embed`). The following files are available:

- [`prices/data.json`](prices/data.json) - JSON file with all prices
- [`prices/data.schema.json`](prices/data.schema.json) - JSON Schema for `prices/data.json`
- [`prices/data_slim.json`](prices/data_slim.json) - JSON file with long fields like descriptions removed and free models removed
- [`prices/data_slim.schema.json`](prices/data_slim.schema.json) - JSON Schema for `prices/data_slim.json`

`prices/data.json` is embedded directly by the Go package (`data_slim.json` is not used).

These files are sourced from upstream [`pydantic/genai-prices`](https://github.com/pydantic/genai-prices);
see the maintainer note above for how updates flow in via Dependabot.

<h2 id="warning">⚠️ Warning: these prices will not be 100% accurate</h2>

This project is a best effort from Pydantic and the community to provide an indicative
estimate of the price you might pay for calling an LLM.

The price data cannot be exactly correct because model providers do not provide exact price information for their APIs
in a format which can be reliably processed.

If you get a bill you weren't expecting, don't blame us!

If you're a lawyer, please read the [LICENSE](LICENSE) under which this project is developed, hosted and distributed.

## Thanks

This project would not be possible without upstream
[`pydantic/genai-prices`](https://github.com/pydantic/genai-prices) and the following existing data sources:

- [Helicone](https://github.com/Helicone/helicone/tree/main/packages/cost)
- [Open Router](https://openrouter.ai/docs/api/api-reference/models/get-models)
- [LiteLLM](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json)
- Simon Willison's [llm-prices](https://github.com/simonw/llm-prices/pull/7)

Thanks to all those projects!
