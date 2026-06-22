package genaiprices

import (
	"fmt"
	"strings"
)

// extractUsage pulls the model name and token usage out of a decoded API
// response for the given API flavor. It mirrors
// packages/js/src/extractUsage.ts:extractUsage.
func extractUsage(provider *Provider, responseData any, apiFlavor string) (string, Usage, error) {
	if apiFlavor == "" {
		apiFlavor = "default"
	}
	if len(provider.Extractors) == 0 {
		return "", Usage{}, fmt.Errorf("genaiprices: no extraction logic defined for provider %q", provider.ID)
	}

	var extractor *UsageExtractor
	for i := range provider.Extractors {
		if provider.Extractors[i].APIFlavor == apiFlavor {
			extractor = &provider.Extractors[i]
			break
		}
	}
	if extractor == nil {
		flavors := make([]string, len(provider.Extractors))
		for i := range provider.Extractors {
			flavors[i] = provider.Extractors[i].APIFlavor
		}
		return "", Usage{}, fmt.Errorf("genaiprices: unknown apiFlavor %q, allowed values: %s",
			apiFlavor, strings.Join(flavors, ", "))
	}

	if !isMapping(responseData) {
		return "", Usage{}, fmt.Errorf("genaiprices: expected response data to be a mapping, got %s", typeName(responseData))
	}

	model, _, err := extractPathString(extractor.ModelPath, responseData, false, nil)
	if err != nil {
		return "", Usage{}, err
	}

	rootValue, _, err := extractPathMapping(extractor.Root, responseData, true, nil)
	if err != nil {
		return "", Usage{}, err
	}

	var usage Usage
	valuesSet := false
	for _, mapping := range extractor.Mappings {
		v, found, err := extractPathNumber(mapping.Path, rootValue, mapping.Required, extractor.Root.Steps)
		if err != nil {
			return "", Usage{}, err
		}
		if found {
			usage.add(mapping.Dest, int(v))
			valuesSet = true
		}
	}
	if !valuesSet {
		return "", Usage{}, fmt.Errorf("genaiprices: no usage information found at %s", pathString(extractor.Root.Steps))
	}
	return model, usage, nil
}

// extractPathString / extractPathMapping / extractPathNumber navigate an
// ExtractPath and return the leaf value with the expected type. found is false
// (with nil error) when the path is absent and not required.
func extractPathString(path ExtractPath, data any, required bool, dataPath []PathStep) (string, bool, error) {
	v, found, err := extractPath(path.Steps, data, "string", required, dataPath)
	if err != nil || !found {
		return "", found, err
	}
	return v.(string), true, nil
}

func extractPathMapping(path ExtractPath, data any, required bool, dataPath []PathStep) (map[string]any, bool, error) {
	v, found, err := extractPath(path.Steps, data, "mapping", required, dataPath)
	if err != nil || !found {
		return nil, found, err
	}
	return v.(map[string]any), true, nil
}

func extractPathNumber(path ExtractPath, data any, required bool, dataPath []PathStep) (float64, bool, error) {
	v, found, err := extractPath(path.Steps, data, "number", required, dataPath)
	if err != nil || !found {
		return 0, found, err
	}
	return v.(float64), true, nil
}

// extractPath walks steps (all but the last must resolve to mappings or arrays)
// and returns the leaf coerced to wantType ("string", "number", "mapping").
func extractPath(steps []PathStep, data any, wantType string, required bool, dataPath []PathStep) (any, bool, error) {
	if len(steps) == 0 {
		return nil, false, fmt.Errorf("genaiprices: empty extract path")
	}
	last := steps[len(steps)-1]
	if last.Array != nil {
		return nil, false, fmt.Errorf("genaiprices: last step of path must be a key, got array-match")
	}
	inner := steps[:len(steps)-1]

	current := data
	var errPath []PathStep
	for _, step := range inner {
		errPath = append(errPath, step)
		if step.Array != nil {
			arr, ok := current.([]any)
			if !ok {
				if !required {
					return nil, false, nil
				}
				return nil, false, fmt.Errorf("genaiprices: expected `%s` value to be a mapping, got %s",
					dottedPath(dataPath, errPath), typeName(current))
			}
			matched := extractArrayMatch(step.Array, arr)
			if matched == nil {
				if required {
					return nil, false, fmt.Errorf("genaiprices: unable to find item at `%s`", dottedPath(dataPath, errPath))
				}
				return nil, false, nil
			}
			current = matched
		} else {
			m, ok := current.(map[string]any)
			if !ok {
				if !required {
					return nil, false, nil
				}
				return nil, false, fmt.Errorf("genaiprices: expected `%s` value to be a mapping, got %s",
					dottedPath(dataPath, errPath), typeName(current))
			}
			next, present := m[step.Key]
			if !present {
				if required {
					return nil, false, fmt.Errorf("genaiprices: missing value at `%s`", dottedPath(dataPath, errPath))
				}
				return nil, false, nil
			}
			current = next
		}
	}

	m, ok := current.(map[string]any)
	if !ok {
		if required {
			return nil, false, fmt.Errorf("genaiprices: expected `%s` value to be a mapping, got %s",
				dottedPath(dataPath, errPath), typeName(current))
		}
		return nil, false, nil
	}
	value, present := m[last.Key]
	if !present {
		if required {
			errPath = append(errPath, last)
			return nil, false, fmt.Errorf("genaiprices: missing value at `%s`", dottedPath(dataPath, errPath))
		}
		return nil, false, nil
	}

	if matchesType(value, wantType) {
		return value, true, nil
	}
	if required {
		errPath = append(errPath, last)
		return nil, false, fmt.Errorf("genaiprices: expected `%s` value to be a %s, got %s",
			dottedPath(dataPath, errPath), wantType, typeName(value))
	}
	return nil, false, nil
}

// extractArrayMatch returns the first mapping item whose Field is a string
// matching the ArrayMatch logic.
func extractArrayMatch(am *ArrayMatch, items []any) map[string]any {
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if field, ok := m[am.Field].(string); ok && am.Match.IsMatch(field) {
			return m
		}
	}
	return nil
}

func isMapping(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

func matchesType(v any, wantType string) bool {
	switch wantType {
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "mapping":
		return isMapping(v)
	default:
		return false
	}
}

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "mapping"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func dottedPath(dataPath, errPath []PathStep) string {
	parts := make([]string, 0, len(dataPath)+len(errPath))
	for _, s := range dataPath {
		parts = append(parts, s.string())
	}
	for _, s := range errPath {
		parts = append(parts, s.string())
	}
	return strings.Join(parts, ".")
}

func pathString(steps []PathStep) string {
	parts := make([]string, len(steps))
	for i, s := range steps {
		parts[i] = s.string()
	}
	return strings.Join(parts, ".")
}

func (s PathStep) string() string {
	if s.Array != nil {
		return fmt.Sprintf("[array-match field=%s]", s.Array.Field)
	}
	return s.Key
}
