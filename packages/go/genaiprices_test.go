package genaiprices_test

import (
	"testing"

	genaiprices "github.com/honeycombio/genai-prices/packages/go"
)

// TestSmoke confirms the package is vendored and wired correctly: the embedded
// data.json catalog parses at init and CalcPrice prices a well-known model.
// This intentionally avoids the upstream test suite; it only guards the embed.
func TestSmoke(t *testing.T) {
	if len(genaiprices.Providers()) == 0 {
		t.Fatal("embedded catalog is empty; data.json failed to load")
	}

	calc, err := genaiprices.CalcPrice(
		genaiprices.Usage{InputTokens: 1000, OutputTokens: 1000},
		"gpt-4o",
		genaiprices.WithProviderID("openai"),
	)
	if err != nil {
		t.Fatalf("CalcPrice(gpt-4o): unexpected error: %v", err)
	}
	if calc.TotalPrice <= 0 {
		t.Fatalf("CalcPrice(gpt-4o): expected positive TotalPrice, got %v", calc.TotalPrice)
	}
}
