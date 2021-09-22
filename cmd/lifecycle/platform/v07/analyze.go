package v07

import (
	"github.com/buildpacks/lifecycle"
)

func (p *v07Platform) AnalyzeOperations() []lifecycle.AnalyzeOperation {
	return []lifecycle.AnalyzeOperation{lifecycle.ReadOptionalPreviousImage}
}
