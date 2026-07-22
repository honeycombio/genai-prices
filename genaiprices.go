// Package genaiprices calculates LLM inference API pricing from an embedded
// catalog of provider and model prices. It is a Go port of the Python and
// JavaScript genai-prices packages and shares their bundled data
// (prices/data.json).
package genaiprices

import (
	"errors"
	"strings"
	"time"
)

// Sentinel errors returned by CalcPrice / FindProvider, matchable with errors.Is.
var (
	ErrProviderNotFound = errors.New("genaiprices: provider not found")
	ErrModelNotFound    = errors.New("genaiprices: model not found")
)

// Usage holds token counts for a single LLM call. All fields are optional;
// InputTokens should INCLUDE cached tokens.
type Usage struct {
	InputTokens          int
	CacheWriteTokens     int
	CacheReadTokens      int
	OutputTokens         int
	InputAudioTokens     int
	CacheAudioReadTokens int
	OutputAudioTokens    int
}

// add increments the field named by an extractor mapping destination.
func (u *Usage) add(dest string, value int) {
	switch dest {
	case "input_tokens":
		u.InputTokens += value
	case "cache_write_tokens":
		u.CacheWriteTokens += value
	case "cache_read_tokens":
		u.CacheReadTokens += value
	case "output_tokens":
		u.OutputTokens += value
	case "input_audio_tokens":
		u.InputAudioTokens += value
	case "cache_audio_read_tokens":
		u.CacheAudioReadTokens += value
	case "output_audio_tokens":
		u.OutputAudioTokens += value
	}
}

// resolveOptions is the internal, resolved option set.
type resolveOptions struct {
	providerID     string
	providerAPIURL string
	provider       *Provider
	timestamp      time.Time
	apiFlavor      string
	modelRef       string // set internally during resolution for model-based provider matching
}

// Option configures CalcPrice, FindProvider and ExtractUsage.
type Option func(*resolveOptions)

// WithProviderID selects the provider by its identifier (e.g. "openai"). The
// special id "litellm" enables "provider/model" prefix handling on the model
// reference.
func WithProviderID(id string) Option {
	return func(o *resolveOptions) { o.providerID = id }
}

// WithProviderAPIURL selects the provider whose api_pattern matches url.
func WithProviderAPIURL(url string) Option {
	return func(o *resolveOptions) { o.providerAPIURL = url }
}

// WithProvider uses the given provider (and only it) instead of the bundled
// catalog, allowing custom or not-yet-published models.
func WithProvider(p *Provider) Option {
	return func(o *resolveOptions) { o.provider = p }
}

// WithTimestamp sets the request time used to select conditional/time-of-day
// prices. Defaults to time.Now().
func WithTimestamp(t time.Time) Option {
	return func(o *resolveOptions) { o.timestamp = t }
}

// WithAPIFlavor selects the extractor flavor for ExtractUsage (default
// "default").
func WithAPIFlavor(flavor string) Option {
	return func(o *resolveOptions) { o.apiFlavor = flavor }
}

func buildOptions(opts []Option) resolveOptions {
	o := resolveOptions{timestamp: time.Now()}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// catalog returns the provider list to resolve against: a single custom
// provider if one was supplied, otherwise the bundled catalog.
func (o resolveOptions) catalog() []Provider {
	if o.provider != nil {
		return []Provider{*o.provider}
	}
	return bundledProviders
}

// CalcPrice calculates the price for usage of modelRef. Provide WithProviderID
// or WithProviderAPIURL when known for the most reliable matching; otherwise
// the model reference is matched against each provider's model_match logic.
//
// It returns ErrProviderNotFound or ErrModelNotFound (matchable with errors.Is)
// when no match exists.
func CalcPrice(usage Usage, modelRef string, opts ...Option) (*PriceCalculation, error) {
	o := buildOptions(opts)
	providers := o.catalog()

	provider, model, err := findProviderModel(providers, modelRef, o)
	if err != nil {
		return nil, err
	}

	mp := getActiveModelPrice(model, o.timestamp)
	input, output, total, err := calcPrice(usage, mp)
	if err != nil {
		return nil, err
	}
	return &PriceCalculation{
		InputPrice:  input,
		OutputPrice: output,
		TotalPrice:  total,
		Provider:    provider,
		Model:       model,
		ModelPrice:  mp,
	}, nil
}

// FindProvider resolves a provider from the given options (WithProviderID,
// WithProviderAPIURL, or WithProvider). It returns ErrProviderNotFound if none
// match.
func FindProvider(opts ...Option) (*Provider, error) {
	o := buildOptions(opts)
	if o.provider != nil {
		return o.provider, nil
	}
	p := matchProvider(bundledProviders, o)
	if p == nil {
		return nil, ErrProviderNotFound
	}
	return p, nil
}

// ExtractUsage extracts the model name and token usage from a decoded API
// response (a map[string]any / []any tree as produced by json.Unmarshal). Pass
// WithAPIFlavor to select a non-default extractor. The returned ExtractedUsage
// includes the matched ModelInfo when the model name resolves within the
// bundled catalog.
func ExtractUsage(provider *Provider, responseData any, opts ...Option) (*ExtractedUsage, error) {
	o := buildOptions(opts)
	modelRef, usage, err := extractUsage(provider, responseData, o.apiFlavor)
	if err != nil {
		return nil, err
	}
	result := &ExtractedUsage{Usage: usage, Provider: provider}
	if modelRef != "" {
		// Resolve the model within this provider (with fallback), ignoring errors:
		// extraction succeeds even if the model isn't in the catalog.
		if m := matchModelWithFallback(provider, strings.ToLower(modelRef), bundledProviders); m != nil {
			result.Model = m
		}
	}
	return result, nil
}

// findProviderModel resolves provider+model, mirroring
// data_snapshot.py:find_provider_model including the litellm "provider/model"
// prefix special case.
func findProviderModel(providers []Provider, modelRef string, o resolveOptions) (*Provider, *ModelInfo, error) {
	modelRef = strings.ToLower(modelRef)
	providerID := o.providerID

	// litellm: extract the real provider from a "provider/model" reference.
	if strings.EqualFold(providerID, "litellm") && strings.Contains(modelRef, "/") {
		parts := strings.SplitN(modelRef, "/", 2)
		actualProviderID, actualModelRef := parts[0], parts[1]
		if actualProviderID != "" && findProviderByID(providers, actualProviderID) != nil {
			providerID = actualProviderID
			modelRef = actualModelRef
			o.providerID = providerID
		}
	}

	var provider *Provider
	if o.provider != nil {
		provider = &providers[0]
	} else {
		o.modelRef = modelRef
		provider = matchProvider(providers, o)
	}
	if provider == nil {
		return nil, nil, ErrProviderNotFound
	}

	model := matchModelWithFallback(provider, modelRef, providers)
	if model == nil {
		return nil, nil, ErrModelNotFound
	}
	return provider, model, nil
}
