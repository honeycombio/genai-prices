package genaiprices

import "time"

// This file holds the pricing engine: provider/model resolution and price
// calculation. The function bodies are stubbed here and implemented in later
// commits so each concern can be reviewed on its own.

func calcPrice(usage Usage, mp ModelPrice) (inputPrice, outputPrice, totalPrice float64, err error) {
	return 0, 0, 0, nil
}

func getActiveModelPrice(model *ModelInfo, ts time.Time) ModelPrice {
	return ModelPrice{}
}

func findProviderByID(providers []Provider, providerID string) *Provider { return nil }

func matchProvider(providers []Provider, o resolveOptions) *Provider { return nil }

func matchModelWithFallback(provider *Provider, modelRef string, allProviders []Provider) *ModelInfo {
	return nil
}
