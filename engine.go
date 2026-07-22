package genaiprices

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// calcMtokPrice computes the price for tokens of one bucket. A nil price means
// the bucket is unpriced (0). Tiered prices use threshold (cliff) pricing keyed
// on totalInputTokens: the highest tier whose start is below the total wins and
// is applied to ALL tokens of this bucket.
func calcMtokPrice(p *Price, tokens int, totalInputTokens int) float64 {
	if p == nil || tokens <= 0 {
		return 0
	}
	if p.Tiered == nil {
		return p.Flat * float64(tokens) / 1_000_000
	}
	applicable := p.Tiered.Base
	for _, tier := range p.Tiered.Tiers {
		if totalInputTokens > tier.Start {
			applicable = tier.Price
		}
	}
	return applicable * float64(tokens) / 1_000_000
}

// calcPrice computes input, output and total prices for usage under modelPrice.
// It mirrors packages/js/src/engine.ts:calcPrice, including the token-bucket
// deduplication that prevents double-charging tokens reported in inclusive
// parent/child buckets (e.g. cached audio counted in input, cache_read and
// input_audio simultaneously).
func calcPrice(usage Usage, mp ModelPrice) (inputPrice, outputPrice, totalPrice float64, err error) {
	totalInputTokens := usage.InputTokens

	cacheReadTokens := usage.CacheReadTokens
	cacheWriteTokens := usage.CacheWriteTokens
	cacheAudioReadTokens := usage.CacheAudioReadTokens
	inputAudioTokens := usage.InputAudioTokens
	outputAudioTokens := usage.OutputAudioTokens

	pricedCacheAudioReadTokens := 0
	if mp.CacheAudioReadMTok != nil {
		pricedCacheAudioReadTokens = cacheAudioReadTokens
	}
	cacheAudioReadAsCacheRead := 0
	if mp.CacheAudioReadMTok == nil && mp.CacheReadMTok != nil {
		cacheAudioReadAsCacheRead = cacheAudioReadTokens
	}

	pricedAudioInputTokens := 0
	if mp.InputAudioMTok != nil {
		pricedAudioInputTokens = inputAudioTokens - pricedCacheAudioReadTokens - cacheAudioReadAsCacheRead
	}
	if pricedAudioInputTokens < 0 {
		return 0, 0, 0, errors.New("genaiprices: cache_audio_read_tokens cannot be greater than input_audio_tokens")
	}

	pricedCacheReadTokens := 0
	if mp.CacheReadMTok != nil {
		pricedCacheReadTokens = cacheReadTokens - pricedCacheAudioReadTokens
	}
	if pricedCacheReadTokens < 0 {
		return 0, 0, 0, errors.New("genaiprices: cache_audio_read_tokens cannot be greater than cache_read_tokens")
	}

	pricedCacheWriteTokens := 0
	if mp.CacheWriteMTok != nil {
		pricedCacheWriteTokens = cacheWriteTokens
	}

	pricedTextInputTokens := 0
	if mp.InputMTok != nil {
		pricedTextInputTokens = totalInputTokens - pricedCacheReadTokens - pricedCacheWriteTokens -
			pricedAudioInputTokens - pricedCacheAudioReadTokens
	}
	if pricedTextInputTokens < 0 {
		return 0, 0, 0, errors.New("genaiprices: uncached text input tokens cannot be negative")
	}

	inputPrice += calcMtokPrice(mp.InputMTok, pricedTextInputTokens, totalInputTokens)
	inputPrice += calcMtokPrice(mp.CacheReadMTok, pricedCacheReadTokens, totalInputTokens)
	inputPrice += calcMtokPrice(mp.CacheWriteMTok, pricedCacheWriteTokens, totalInputTokens)
	inputPrice += calcMtokPrice(mp.InputAudioMTok, pricedAudioInputTokens, totalInputTokens)
	inputPrice += calcMtokPrice(mp.CacheAudioReadMTok, pricedCacheAudioReadTokens, totalInputTokens)

	pricedTextOutputTokens := 0
	if mp.OutputMTok != nil {
		pricedTextOutputTokens = usage.OutputTokens
		if mp.OutputAudioMTok != nil {
			pricedTextOutputTokens -= outputAudioTokens
		}
	}
	if pricedTextOutputTokens < 0 {
		return 0, 0, 0, errors.New("genaiprices: output_audio_tokens cannot be greater than output_tokens")
	}
	outputPrice += calcMtokPrice(mp.OutputMTok, pricedTextOutputTokens, totalInputTokens)
	outputPrice += calcMtokPrice(mp.OutputAudioMTok, outputAudioTokens, totalInputTokens)

	totalPrice = inputPrice + outputPrice
	if mp.RequestsKCount != nil {
		totalPrice += *mp.RequestsKCount / 1000
	}
	return inputPrice, outputPrice, totalPrice, nil
}

// getActiveModelPrice selects the active price for a model at ts. Conditional
// prices are tried last to first; a nil constraint or a satisfied constraint
// wins. If none match, the first price is used as a fallback.
func getActiveModelPrice(model *ModelInfo, ts time.Time) ModelPrice {
	prices := model.Prices
	if len(prices) == 0 {
		return ModelPrice{}
	}
	for i := len(prices) - 1; i >= 0; i-- {
		cond := prices[i]
		c := cond.Constraint
		if c == nil {
			return cond.Prices
		}
		if c.Kind == "start_date" {
			if !ts.Before(c.StartDate) {
				return cond.Prices
			}
			continue
		}
		// time_of_date: compare UTC "HH:MM:SS", handling midnight wrap.
		t := ts.UTC().Format("15:04:05")
		if c.EndTime < c.StartTime {
			if t >= c.StartTime || t < c.EndTime {
				return cond.Prices
			}
		} else {
			if t >= c.StartTime && t < c.EndTime {
				return cond.Prices
			}
		}
	}
	return prices[0].Prices
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
