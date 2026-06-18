package genaiprices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Provider is an LLM inference provider together with its models and the logic
// used to match it and extract usage from its API responses.
type Provider struct {
	ID                     string           `json:"id"`
	Name                   string           `json:"name"`
	APIPattern             string           `json:"api_pattern"`
	PricingURLs            []string         `json:"pricing_urls,omitempty"`
	Description            string           `json:"description,omitempty"`
	PriceComments          string           `json:"price_comments,omitempty"`
	ModelMatch             *MatchLogic      `json:"model_match,omitempty"`
	ProviderMatch          *MatchLogic      `json:"provider_match,omitempty"`
	Extractors             []UsageExtractor `json:"extractors,omitempty"`
	FallbackModelProviders []string         `json:"fallback_model_providers,omitempty"`
	Models                 []ModelInfo      `json:"models"`
}

// ModelInfo is a single model offered by a provider.
type ModelInfo struct {
	ID            string     `json:"id"`
	Match         MatchLogic `json:"match"`
	Name          string     `json:"name,omitempty"`
	Description   string     `json:"description,omitempty"`
	ContextWindow *int       `json:"context_window,omitempty"`
	PriceComments string     `json:"price_comments,omitempty"`
	Deprecated    bool       `json:"deprecated,omitempty"`

	// Prices is always normalized to a slice of conditional prices. A bare
	// ModelPrice object in the source data becomes a single entry with a nil
	// constraint. See getActiveModelPrice for how an active price is selected.
	Prices []ConditionalPrice `json:"prices"`
}

// UnmarshalJSON normalizes the polymorphic `prices` field (either a single
// ModelPrice object or an array of ConditionalPrice) into a slice.
func (m *ModelInfo) UnmarshalJSON(data []byte) error {
	type rawModel struct {
		ID            string          `json:"id"`
		Match         MatchLogic      `json:"match"`
		Name          string          `json:"name"`
		Description   string          `json:"description"`
		ContextWindow *int            `json:"context_window"`
		PriceComments string          `json:"price_comments"`
		Deprecated    bool            `json:"deprecated"`
		Prices        json.RawMessage `json:"prices"`
	}
	var raw rawModel
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.ID = raw.ID
	m.Match = raw.Match
	m.Name = raw.Name
	m.Description = raw.Description
	m.ContextWindow = raw.ContextWindow
	m.PriceComments = raw.PriceComments
	m.Deprecated = raw.Deprecated

	if len(raw.Prices) == 0 {
		return nil
	}
	switch firstToken(raw.Prices) {
	case '[':
		return json.Unmarshal(raw.Prices, &m.Prices)
	case '{':
		var mp ModelPrice
		if err := json.Unmarshal(raw.Prices, &mp); err != nil {
			return err
		}
		m.Prices = []ConditionalPrice{{Prices: mp}}
		return nil
	default:
		return fmt.Errorf("genaiprices: unexpected `prices` value for model %q", raw.ID)
	}
}

// ConditionalPrice pairs a set of prices with an optional constraint defining
// when those prices apply.
type ConditionalPrice struct {
	Constraint *Constraint `json:"constraint,omitempty"`
	Prices     ModelPrice  `json:"prices"`
}

// Constraint defines when a ConditionalPrice is active. The source data
// distinguishes the two kinds by which fields are present: a start_date marks a
// date constraint; start_time + end_time mark a daily time-of-day window.
type Constraint struct {
	// Kind is either "start_date" or "time_of_date".
	Kind string

	// StartDate is set when Kind == "start_date".
	StartDate time.Time

	// StartTime / EndTime are "HH:MM:SS" UTC strings when Kind == "time_of_date".
	StartTime string
	EndTime   string
}

func (c *Constraint) UnmarshalJSON(data []byte) error {
	var raw struct {
		StartDate string `json:"start_date"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch {
	case raw.StartDate != "":
		t, err := time.Parse("2006-01-02", raw.StartDate)
		if err != nil {
			return fmt.Errorf("genaiprices: invalid start_date %q: %w", raw.StartDate, err)
		}
		c.Kind = "start_date"
		c.StartDate = t
	case raw.StartTime != "" && raw.EndTime != "":
		c.Kind = "time_of_date"
		c.StartTime = stripZ(raw.StartTime)
		c.EndTime = stripZ(raw.EndTime)
	default:
		return fmt.Errorf("genaiprices: unrecognized constraint %s", string(data))
	}
	return nil
}

// stripZ removes a trailing "Z" from a "HH:MM:SSZ" time so it can be compared
// directly against time.Time.UTC().Format("15:04:05").
func stripZ(s string) string {
	return strings.TrimSuffix(s, "Z")
}

// ModelPrice is the set of per-token (per million) prices for a model. A nil
// pointer field means that bucket is not priced, which the engine relies on.
type ModelPrice struct {
	InputMTok          *Price   `json:"input_mtok,omitempty"`
	CacheWriteMTok     *Price   `json:"cache_write_mtok,omitempty"`
	CacheReadMTok      *Price   `json:"cache_read_mtok,omitempty"`
	OutputMTok         *Price   `json:"output_mtok,omitempty"`
	InputAudioMTok     *Price   `json:"input_audio_mtok,omitempty"`
	CacheAudioReadMTok *Price   `json:"cache_audio_read_mtok,omitempty"`
	OutputAudioMTok    *Price   `json:"output_audio_mtok,omitempty"`
	RequestsKCount     *float64 `json:"requests_kcount,omitempty"`
}

// Price is a per-million-token price that is either a flat rate or tiered.
type Price struct {
	// Flat is the price per million tokens when Tiered is nil.
	Flat float64
	// Tiered, when non-nil, defines threshold (cliff) pricing.
	Tiered *TieredPrices
}

func (p *Price) UnmarshalJSON(data []byte) error {
	switch firstToken(data) {
	case '{':
		var t TieredPrices
		if err := json.Unmarshal(data, &t); err != nil {
			return err
		}
		p.Tiered = &t
		return nil
	default:
		return json.Unmarshal(data, &p.Flat)
	}
}

// TieredPrices is threshold-based (cliff) pricing: crossing a tier applies that
// tier's rate to ALL tokens.
type TieredPrices struct {
	Base  float64 `json:"base"`
	Tiers []Tier  `json:"tiers"`
}

func (t *TieredPrices) UnmarshalJSON(data []byte) error {
	type alias TieredPrices
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*t = TieredPrices(a)
	// Sort tiers ascending by start, matching the Python/JS implementations.
	sort.SliceStable(t.Tiers, func(i, j int) bool { return t.Tiers[i].Start < t.Tiers[j].Start })
	return nil
}

// Tier is a single price tier in TieredPrices.
type Tier struct {
	Start int     `json:"start"`
	Price float64 `json:"price"`
}

// UsageExtractor describes how to pull usage and the model name out of a
// provider API response for a given API flavor.
type UsageExtractor struct {
	APIFlavor string                  `json:"api_flavor"`
	Root      ExtractPath             `json:"root"`
	ModelPath ExtractPath             `json:"model_path"`
	Mappings  []UsageExtractorMapping `json:"mappings"`
}

// UsageExtractorMapping maps a path in the response to a Usage field.
type UsageExtractorMapping struct {
	Path     ExtractPath `json:"path"`
	Dest     string      `json:"dest"`
	Required bool        `json:"required"`
}

// ExtractPath is a path into a decoded JSON response: a sequence of object keys
// (strings) and ArrayMatch steps. The source encodes it as either a single
// string or an array of strings/ArrayMatch objects.
type ExtractPath struct {
	Steps []PathStep
}

func (e *ExtractPath) UnmarshalJSON(data []byte) error {
	switch firstToken(data) {
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		e.Steps = []PathStep{{Key: s}}
		return nil
	case '[':
		return json.Unmarshal(data, &e.Steps)
	default:
		return fmt.Errorf("genaiprices: unexpected extract path %s", string(data))
	}
}

// PathStep is one step of an ExtractPath: either an object key or an ArrayMatch.
type PathStep struct {
	Key   string
	Array *ArrayMatch
}

func (s *PathStep) UnmarshalJSON(data []byte) error {
	switch firstToken(data) {
	case '"':
		return json.Unmarshal(data, &s.Key)
	case '{':
		var am ArrayMatch
		if err := json.Unmarshal(data, &am); err != nil {
			return err
		}
		s.Array = &am
		return nil
	default:
		return fmt.Errorf("genaiprices: unexpected path step %s", string(data))
	}
}

// ArrayMatch finds the first item in an array whose Field matches Match.
type ArrayMatch struct {
	Type  string     `json:"type"`
	Field string     `json:"field"`
	Match MatchLogic `json:"match"`
}

// PriceCalculation is the result of CalcPrice.
type PriceCalculation struct {
	InputPrice  float64
	OutputPrice float64
	TotalPrice  float64
	Provider    *Provider
	Model       *ModelInfo
	ModelPrice  ModelPrice
}

// ExtractedUsage is the result of ExtractUsage.
type ExtractedUsage struct {
	Usage    Usage
	Model    *ModelInfo
	Provider *Provider
}

// firstToken returns the first non-whitespace byte of a JSON message, or 0.
func firstToken(data []byte) byte {
	data = bytes.TrimLeft(data, " \t\r\n")
	if len(data) == 0 {
		return 0
	}
	return data[0]
}
