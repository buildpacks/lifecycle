package v06

import (
	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform/pre06"
)

func (p *v06Platform) AnalyzeOperations() []lifecycle.AnalyzeOperation {
	return []lifecycle.AnalyzeOperation{pre06.ReadPreviousImage, pre06.RestoreLayerMetadata}
}
