package patch

import (
	"path/filepath"
	"strings"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform/files"
)

// LayerMatcher provides methods for matching buildpack layers against patch selectors.
type LayerMatcher struct{}

// NewLayerMatcher creates a new LayerMatcher.
func NewLayerMatcher() *LayerMatcher {
	return &LayerMatcher{}
}

// MatchResult represents a matched layer within the metadata.
type MatchResult struct {
	BuildpackIndex int
	BuildpackID    string
	LayerName      string
	LayerMetadata  buildpack.LayerMetadata
}

// FindMatchingLayers finds all layers in the metadata that match the given patch.
// A patch can match multiple layers if the buildpack appears multiple times or
// if glob patterns match multiple layer names/data values.
func (m *LayerMatcher) FindMatchingLayers(metadata files.LayersMetadataCompat, patch files.LayerPatch) []MatchResult {
	var results []MatchResult

	for bpIdx, bp := range metadata.Buildpacks {
		// Check if buildpack matches (using glob pattern)
		if !matchGlob(patch.Buildpack, bp.ID) {
			continue
		}

		for layerName, layerMD := range bp.Layers {
			// Check if layer name matches (using glob pattern)
			if !matchGlob(patch.Layer, layerName) {
				continue
			}

			// Check if data selectors match
			if !m.matchData(layerMD.Data, patch.Data) {
				continue
			}

			results = append(results, MatchResult{
				BuildpackIndex: bpIdx,
				BuildpackID:    bp.ID,
				LayerName:      layerName,
				LayerMetadata:  layerMD,
			})
		}
	}

	return results
}

// matchData checks if the layer data matches all the selectors in the patch.
// Selectors use dot-notation to navigate nested structures and support glob patterns.
func (m *LayerMatcher) matchData(layerData interface{}, selectors map[string]string) bool {
	if len(selectors) == 0 {
		return true
	}

	if layerData == nil {
		return false
	}

	for path, pattern := range selectors {
		value, found := getNestedValue(layerData, path)
		if !found {
			return false
		}
		if !matchGlob(pattern, value) {
			return false
		}
	}

	return true
}

// getNestedValue traverses a nested map structure using dot-notation path.
// For example, "artifact.version" will navigate data["artifact"]["version"].
func getNestedValue(data interface{}, path string) (string, bool) {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			next, ok := v[part]
			if !ok {
				return "", false
			}
			current = next
		case map[interface{}]interface{}:
			next, ok := v[part]
			if !ok {
				return "", false
			}
			current = next
		default:
			return "", false
		}
	}

	// Convert final value to string
	switch v := current.(type) {
	case string:
		return v, true
	case int:
		return strings.TrimSpace(strings.Repeat(" ", v)), false // fallback for non-string
	case float64:
		// Handle numeric values
		return "", false
	default:
		return "", false
	}
}

// matchGlob performs glob pattern matching.
// Supports patterns like "1.2.*" matching "1.2.0", "1.2.3", etc.
func matchGlob(pattern, value string) bool {
	if pattern == "" {
		return value == ""
	}

	// Use filepath.Match for glob support
	matched, err := filepath.Match(pattern, value)
	if err != nil {
		// If the pattern is invalid, fall back to exact match
		return pattern == value
	}
	return matched
}
