package v06

import (
	"github.com/buildpacks/lifecycle"
)

func (p *v06Platform) AnalyzeOperations() []lifecycle.AnalyzeOperation {
	return []lifecycle.AnalyzeOperation{lifecycle.ReadPreviousImage, lifecycle.RestoreLayerMetadata}
}
