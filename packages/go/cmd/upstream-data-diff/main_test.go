package main

import (
	"reflect"
	"testing"

	genaiprices "github.com/honeycombio/genai-prices/packages/go"
)

// priced builds a single-entry conditional price list with the given flat
// input price, so two models can be made to differ semantically.
func priced(flat float64) []genaiprices.ConditionalPrice {
	return []genaiprices.ConditionalPrice{{
		Prices: genaiprices.ModelPrice{InputMTok: &genaiprices.Price{Flat: flat}},
	}}
}

func model(id string, flat float64) genaiprices.ModelInfo {
	return genaiprices.ModelInfo{ID: id, Prices: priced(flat)}
}

func TestDiffCatalog(t *testing.T) {
	old := []genaiprices.Provider{
		{ID: "anthropic", Models: []genaiprices.ModelInfo{
			model("claude", 1.0),  // unchanged
			model("opus", 2.0),    // price changes -> changed
			model("legacy", 0.5),  // removed
		}},
		{ID: "openai", Models: []genaiprices.ModelInfo{model("gpt", 3.0)}}, // provider removed
	}
	newV := []genaiprices.Provider{
		{ID: "anthropic", Models: []genaiprices.ModelInfo{
			model("claude", 1.0),    // unchanged
			model("opus", 2.5),      // changed
			model("haiku", 0.25),    // added
		}},
		{ID: "voyageai", Models: []genaiprices.ModelInfo{model("voyage-3", 0.1)}}, // provider added
	}

	got := diffCatalog(old, newV)
	want := CatalogDiff{
		ProvidersAdded:   []string{"voyageai"},
		ProvidersRemoved: []string{"openai"},
		ModelsAdded:      []string{"anthropic/haiku", "voyageai/voyage-3"},
		ModelsRemoved:    []string{"anthropic/legacy", "openai/gpt"},
		ModelsChanged:    []string{"anthropic/opus"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("diffCatalog mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestDiffCatalogNoChange(t *testing.T) {
	provs := []genaiprices.Provider{
		{ID: "anthropic", Models: []genaiprices.ModelInfo{model("claude", 1.0)}},
	}
	// Distinct slices with equal content must report no change.
	clone := []genaiprices.Provider{
		{ID: "anthropic", Models: []genaiprices.ModelInfo{model("claude", 1.0)}},
	}
	if got := diffCatalog(provs, clone); !got.empty() {
		t.Errorf("expected empty diff, got %+v", got)
	}
}

func TestModelKeyFallsBackToName(t *testing.T) {
	// A model with no ID is keyed by its Name (mirrors the original behavior).
	provs := []genaiprices.Provider{
		{ID: "p", Models: []genaiprices.ModelInfo{{Name: "named-model", Prices: priced(1)}}},
	}
	m := modelMap(provs)
	if _, ok := m["p/named-model"]; !ok {
		t.Errorf("expected key p/named-model, got keys %v", keys(m))
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestParseArgs(t *testing.T) {
	old, newV, pr, err := parseArgs([]string{"0.0.66", "0.0.67", "--pr", "42"})
	if err != nil || old != "0.0.66" || newV != "0.0.67" || pr != "42" {
		t.Fatalf("got (%q,%q,%q,%v)", old, newV, pr, err)
	}
	if _, _, _, err := parseArgs([]string{"0.0.66"}); err == nil {
		t.Errorf("expected error for missing new version")
	}
	if _, _, _, err := parseArgs([]string{"a", "b", "--pr"}); err == nil {
		t.Errorf("expected error for --pr without value")
	}
}
