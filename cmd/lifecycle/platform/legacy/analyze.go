package legacy

import (
	"github.com/buildpacks/lifecycle"
)

func (p *legacyPlatform) AnalyzeOperations() []lifecycle.AnalyzeOperation {
	return []lifecycle.AnalyzeOperation{lifecycle.ReadPreviousImage, lifecycle.RestoreLayerMetadata}
}
