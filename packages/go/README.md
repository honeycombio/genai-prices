# genai-prices (Go)

Go package for calculating LLM inference API prices, a port of the Python and
JavaScript [`genai-prices`](https://github.com/pydantic/genai-prices) packages.
It embeds upstream's price catalog (`data.json`) at compile time via `//go:embed`.

## Install

```bash
go get github.com/honeycombio/genai-prices/packages/go
```

```go
import genaiprices "github.com/honeycombio/genai-prices/packages/go"
```

> This package currently lives in the `honeycombio` fork. The module path will
> change to `github.com/pydantic/genai-prices/packages/go` if/when it is merged
> upstream.

## Usage

### Calculate a price

```go
usage := genaiprices.Usage{InputTokens: 1000, OutputTokens: 100}

calc, err := genaiprices.CalcPrice(usage, "gpt-4o-mini",
    genaiprices.WithProviderID("openai"))
if err != nil {
    // errors.Is(err, genaiprices.ErrProviderNotFound) / ErrModelNotFound
    log.Fatal(err)
}

fmt.Printf("$%.6f (input $%.6f, output $%.6f) — %s / %s\n",
    calc.TotalPrice, calc.InputPrice, calc.OutputPrice,
    calc.Provider.Name, calc.Model.Name)
```

Provide `WithProviderID` (or `WithProviderAPIURL`) when you know the provider for
the most reliable matching. Without it, the model reference is matched against
each provider's `model_match` logic.

### Options

- `WithProviderID(id)` — select provider by id (e.g. `"openai"`). The special id
  `"litellm"` enables `provider/model` prefix handling on the model reference.
- `WithProviderAPIURL(url)` — select provider whose `api_pattern` matches `url`.
- `WithProvider(p)` — use a custom provider (and only it) instead of the bundled
  catalog, for models not yet published.
- `WithTimestamp(t)` — request time used to pick conditional / time-of-day
  prices (defaults to `time.Now()`).
- `WithAPIFlavor(flavor)` — extractor flavor for `ExtractUsage` (default
  `"default"`).

### Custom / unpublished models

```go
custom := &genaiprices.Provider{
    ID: "custom", Name: "Custom", APIPattern: ".*",
    Models: []genaiprices.ModelInfo{{
        ID:    "my-model",
        Match: genaiprices.Equals("my-model"),
        Prices: []genaiprices.ConditionalPrice{{Prices: genaiprices.ModelPrice{
            InputMTok:  &genaiprices.Price{Flat: 2.5},
            OutputMTok: &genaiprices.Price{Flat: 10.0},
        }}},
    }},
}
calc, err := genaiprices.CalcPrice(usage, "my-model", genaiprices.WithProvider(custom))
```

### Extract usage from an API response

```go
var responseData any
_ = json.Unmarshal(rawResponseBody, &responseData)

provider, _ := genaiprices.FindProvider(genaiprices.WithProviderID("anthropic"))
extracted, err := genaiprices.ExtractUsage(provider, responseData)
if err != nil {
    log.Fatal(err)
}
calc, _ := genaiprices.CalcPrice(extracted.Usage, extracted.Model.ID,
    genaiprices.WithProvider(provider))
```

## Notes

- Prices use `float64`, matching the JavaScript engine.
- Tiered pricing is threshold-based (cliff): crossing a tier applies that rate to
  all tokens of that bucket.
- `data.json` is vendored from upstream — DO NOT edit it directly. Refresh it with
  [`sync-upstream-data.sh`](./sync-upstream-data.sh) (see [SYNCING.md](./SYNCING.md)).
