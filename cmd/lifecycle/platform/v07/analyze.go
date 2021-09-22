package v07

import (
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/platform"
)

func (p *v07Platform) AnalyzeOperations() []lifecycle.AnalyzeOperation {
	return []lifecycle.AnalyzeOperation{ReadOptionalPreviousImage}
}

func ReadOptionalPreviousImage(a *lifecycle.Analyzer, analyzedMD *platform.AnalyzedMetadata) error {
	if a.Image == nil {
		return nil
	}

	var err error
	analyzedMD.Image, err = a.GetImageIdentifier(a.Image)
	if err != nil {
		return errors.Wrap(err, "retrieving image identifier")
	}

	_ = lifecycle.DecodeLabel(a.Image, platform.LayerMetadataLabel, &analyzedMD.Metadata) // continue even if the label cannot be decoded
	return nil
}
