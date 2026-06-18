package genaiprices

import (
	"regexp"
	"strings"
	"time"
)

// calcPrice / getActiveModelPrice are stubbed here; price calculation is
// implemented in a following commit. This commit covers provider/model
// resolution.

func calcPrice(usage Usage, mp ModelPrice) (inputPrice, outputPrice, totalPrice float64, err error) {
	return 0, 0, 0, nil
}

func getActiveModelPrice(model *ModelInfo, ts time.Time) ModelPrice {
	return ModelPrice{}
}

// findProviderByID resolves a provider by its id (exact, normalized) and then
// by its provider_match logic.
func findProviderByID(providers []Provider, providerID string) *Provider {
	normalized := strings.TrimSpace(strings.ToLower(providerID))
	for i := range providers {
		if providers[i].ID == normalized {
			return &providers[i]
		}
	}
	for i := range providers {
		if providers[i].ProviderMatch != nil && providers[i].ProviderMatch.IsMatch(normalized) {
			return &providers[i]
		}
	}
	return nil
}

// matchProvider finds a provider given an explicit id, an API URL, or a model
// reference (in that priority order). The litellm provider id falls through to
// model matching when no provider with that id exists.
func matchProvider(providers []Provider, o resolveOptions) *Provider {
	if o.providerID != "" {
		p := findProviderByID(providers, o.providerID)
		if p != nil || strings.ToLower(o.providerID) != "litellm" {
			return p
		}
		// litellm: fall through to model matching
	}
	if o.providerAPIURL != "" {
		for i := range providers {
			if re := compileRegex(providers[i].APIPattern); re != nil && matchAnchoredStart(re, o.providerAPIURL) {
				return &providers[i]
			}
		}
		return nil
	}
	if o.modelRef != "" {
		for i := range providers {
			if providers[i].ModelMatch != nil && providers[i].ModelMatch.IsMatch(o.modelRef) {
				return &providers[i]
			}
		}
	}
	return nil
}

// matchAnchoredStart mirrors Python's re.match (anchored at the start) used for
// api_pattern URL matching. The JS implementation uses RegExp.test (unanchored)
// but every api_pattern in the dataset begins with a scheme, so anchoring at the
// start is the stricter, correct interpretation matching the Python package.
func matchAnchoredStart(re *regexp.Regexp, s string) bool {
	loc := re.FindStringIndex(s)
	return loc != nil && loc[0] == 0
}

// matchModel returns the first model whose match logic matches modelRef.
func matchModel(models []ModelInfo, modelRef string) *ModelInfo {
	for i := range models {
		if models[i].Match.IsMatch(modelRef) {
			return &models[i]
		}
	}
	return nil
}

// matchModelWithFallback tries the provider's own models, then one level of
// fallback_model_providers (e.g. AWS/Azure offering Anthropic/OpenAI models).
func matchModelWithFallback(provider *Provider, modelRef string, allProviders []Provider) *ModelInfo {
	if m := matchModel(provider.Models, modelRef); m != nil {
		return m
	}
	if len(provider.FallbackModelProviders) > 0 && allProviders != nil {
		for _, fid := range provider.FallbackModelProviders {
			for i := range allProviders {
				if allProviders[i].ID == fid {
					// Do not pass allProviders again: only one level of fallback.
					if m := matchModelWithFallback(&allProviders[i], modelRef, nil); m != nil {
						return m
					}
				}
			}
		}
	}
	return nil
}
