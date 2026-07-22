package genaiprices

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// dataJSON is the bundled price catalog, embedded directly from
// prices/data.json. DO NOT EDIT it directly — it's synced from upstream, see
// SYNCING.md.
//
//go:embed prices/data.json
var dataJSON []byte

// bundledProviders is the parsed catalog, populated once at init.
var bundledProviders []Provider

func init() {
	if err := json.Unmarshal(dataJSON, &bundledProviders); err != nil {
		panic(fmt.Sprintf("genaiprices: failed to parse embedded data.json: %v", err))
	}
}

// Providers returns the bundled price catalog. The returned slice is shared;
// treat it as read-only.
func Providers() []Provider {
	return bundledProviders
}
