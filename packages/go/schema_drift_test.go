package genaiprices_test

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	genaiprices "github.com/honeycombio/genai-prices/packages/go"
)

// knownKeys is every JSON object key the hand-written Go structs understand,
// across the whole catalog schema (Provider, ModelInfo, ModelPrice, Constraint,
// TieredPrices/Tier, UsageExtractor/Mapping, ArrayMatch, and MatchLogic
// operators). It is the contract between prices/data.json and packages/go/*.go.
//
// When syncing upstream, `make build` regenerates packages/go/data.json. If
// upstream changes the data FORMAT (adds, renames, or moves a field), this set
// will no longer match the keys present in data.json and TestSchemaDrift fails —
// turning an otherwise SILENT mismatch (Go's json.Unmarshal ignores unknown
// fields and zero-values renamed ones) into a loud CI failure on the sync PR.
//
// To resolve a failure: reconcile packages/go/types.go (+ match.go/extract.go)
// with the new shape shown by `git diff prices/data.schema.json`, then update
// this set to match. See SYNCING.md.
var knownKeys = map[string]bool{
	// Provider
	"id": true, "name": true, "api_pattern": true, "pricing_urls": true,
	"description": true, "price_comments": true, "model_match": true,
	"provider_match": true, "extractors": true, "fallback_model_providers": true,
	"models": true,
	// ModelInfo
	"match": true, "context_window": true, "deprecated": true, "prices": true,
	// ConditionalPrice / Constraint
	"constraint": true, "start_date": true, "start_time": true, "end_time": true,
	// ModelPrice
	"input_mtok": true, "cache_write_mtok": true, "cache_read_mtok": true,
	"output_mtok": true, "input_audio_mtok": true, "cache_audio_read_mtok": true,
	"output_audio_mtok": true, "requests_kcount": true,
	// TieredPrices / Tier
	"base": true, "tiers": true, "start": true, "price": true,
	// UsageExtractor / Mapping / ArrayMatch
	"api_flavor": true, "root": true, "model_path": true, "mappings": true,
	"path": true, "dest": true, "required": true, "type": true, "field": true,
	// MatchLogic operators
	"equals": true, "starts_with": true, "ends_with": true, "contains": true,
	"regex": true, "or": true, "and": true,
}

// collectKeys recursively gathers every object key in a decoded JSON tree.
func collectKeys(v any, into map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			into[k] = true
			collectKeys(child, into)
		}
	case []any:
		for _, child := range t {
			collectKeys(child, into)
		}
	}
}

// TestSchemaDrift fails if the embedded data.json contains any object key the Go
// structs don't model (an addition or rename upstream) or is missing a key the
// structs expect. Either direction means the data format drifted out from under
// the hand-written Go types. The Python/JS packages are schema-validated at
// build time; this is the equivalent guard for Go.
func TestSchemaDrift(t *testing.T) {
	raw, err := os.ReadFile("data.json")
	if err != nil {
		t.Fatalf("read data.json: %v", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("parse data.json: %v", err)
	}

	seen := map[string]bool{}
	collectKeys(tree, seen)

	var unexpected, missing []string
	for k := range seen {
		if !knownKeys[k] {
			unexpected = append(unexpected, k)
		}
	}
	for k := range knownKeys {
		if !seen[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(unexpected)
	sort.Strings(missing)

	if len(unexpected) > 0 {
		t.Errorf("data.json has keys not modeled by the Go structs: %v\n"+
			"upstream likely changed the data format. Reconcile packages/go/types.go "+
			"(see `git diff prices/data.schema.json`) and update knownKeys. See SYNCING.md.",
			unexpected)
	}
	if len(missing) > 0 {
		t.Errorf("Go structs expect keys absent from data.json (renamed/removed upstream?): %v\n"+
			"reconcile packages/go/types.go and update knownKeys. See SYNCING.md.", missing)
	}
}

// goldenModels are stable, long-lived models used to assert pricing still
// resolves to non-zero values. We deliberately do NOT assert exact prices —
// those change on every routine upstream sync. We assert the buckets are
// populated and consistent, which catches a renamed price field decoding to a
// silent zero (Go's json.Unmarshal zero-values unknown tags).
var goldenModels = []struct {
	providerID string
	modelRef   string
}{
	{"openai", "gpt-4o"},
	{"anthropic", "claude-3-5-sonnet-latest"},
}

func TestGoldenPricesNonZero(t *testing.T) {
	usage := genaiprices.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	for _, g := range goldenModels {
		t.Run(g.providerID+"/"+g.modelRef, func(t *testing.T) {
			calc, err := genaiprices.CalcPrice(usage, g.modelRef, genaiprices.WithProviderID(g.providerID))
			if err != nil {
				t.Fatalf("CalcPrice: unexpected error: %v", err)
			}
			if calc.InputPrice <= 0 {
				t.Errorf("InputPrice = %v, want > 0 (input_mtok may have been renamed/dropped)", calc.InputPrice)
			}
			if calc.OutputPrice <= 0 {
				t.Errorf("OutputPrice = %v, want > 0 (output_mtok may have been renamed/dropped)", calc.OutputPrice)
			}
			if got, want := calc.TotalPrice, calc.InputPrice+calc.OutputPrice; got != want {
				t.Errorf("TotalPrice = %v, want InputPrice+OutputPrice = %v", got, want)
			}
		})
	}
}
