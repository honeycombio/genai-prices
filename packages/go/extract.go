package genaiprices

// extractUsage pulls the model name and token usage out of a decoded API
// response. Stubbed here; the extraction logic is added in a later commit.
func extractUsage(provider *Provider, responseData any, apiFlavor string) (string, Usage, error) {
	return "", Usage{}, nil
}
