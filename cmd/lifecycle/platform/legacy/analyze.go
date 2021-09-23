package legacy

import (
	"github.com/buildpacks/lifecycle"
)

func (p *pre06Platform) AnalyzeOperations() []lifecycle.AnalyzeOperation {
	return []lifecycle.AnalyzeOperation{lifecycle.ReadPreviousImage, lifecycle.RestoreLayerMetadata}
}
