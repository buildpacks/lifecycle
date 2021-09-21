package common

import (
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/platform"
)

func ReadPreviousImage(a *lifecycle.Analyzer, analyzedMD *platform.AnalyzedMetadata) error {
	if a.Image == nil { // Image is optional in Platform API >= 0.7
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

func RestoreLayerMetadata(a *lifecycle.Analyzer, analyzedMD *platform.AnalyzedMetadata) error {
	cacheMeta, err := lifecycle.RetrieveCacheMetadata(a.Cache, a.Logger)
	if err != nil {
		return err
	}

	useShaFiles := true
	if err := a.LayerMetadataRestorer.Restore(a.Buildpacks, analyzedMD.Metadata, cacheMeta, lifecycle.NewLayerSHAStore(useShaFiles)); err != nil {
		return err
	}

	return nil
}
