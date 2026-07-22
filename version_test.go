package genaiprices

import (
	"regexp"
	"testing"
)

var semver = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// TestVersionConstants guards the provenance identifiers consumers stamp onto
// cost estimates.
func TestVersionConstants(t *testing.T) {
	if Name != "genai-prices" {
		t.Errorf("Name = %q, want %q", Name, "genai-prices")
	}
	if DataSource != "pydantic/genai-prices" {
		t.Errorf("DataSource = %q, want %q", DataSource, "pydantic/genai-prices")
	}
	if !semver.MatchString(Version) {
		t.Errorf("Version = %q, want a semver-like release version", Version)
	}
}
