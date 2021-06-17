package v07

import (
	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/platform/common"
)

func (p *Platform) DecodeAnalyzedMetadataFile(path string) (common.AnalyzedMetadata, error) {
	var (
		analyzedMd analyzedMetadata // TODO: change analyzedMD to analyzedMd
		err        error
	)

	if _, err = toml.DecodeFile(path, &analyzedMd); err == nil {
		return &analyzedMd, nil
	}
	return nil, err
}

func (p *Platform) NewAnalyzedMetadata(config common.AnalyzedMetadataConfig) common.AnalyzedMetadata {
	return &analyzedMetadata{
		BuildImageData: BuildImage{
			StackID: config.BuildImageStackID,
			Mixins:  config.BuildImageMixins,
		},
		PreviousImageData: PreviousImage{
			Reference: config.PreviousImage,
			Metadata:  config.PreviousImageMetadata,
		},
		RunImageData: RunImage{
			Reference: config.RunImage,
			Mixins:    config.RunImageMixins,
		},
	}
}

// AnalyzedMetadata

type analyzedMetadata struct {
	BuildImageData    BuildImage    `toml:"build-image"`
	PreviousImageData PreviousImage `toml:"previous-image"`
	RunImageData      RunImage      `toml:"run-image"`
}

type BuildImage struct {
	StackID string   `toml:"stack-id"`
	Mixins  []string `toml:"mixins"`
}

type PreviousImage struct {
	Reference *common.ImageIdentifier `toml:"reference"`
	Metadata  common.LayersMetadata   `toml:"metadata"`
}

type RunImage struct {
	Reference *common.ImageIdentifier `toml:"reference"`
	Mixins    []string                `toml:"mixins"`
}

func (a *analyzedMetadata) BuildImageStackID() string {
	return a.BuildImageData.StackID
}

func (a *analyzedMetadata) BuildImageMixins() []string {
	return a.BuildImageData.Mixins
}

func (a *analyzedMetadata) PreviousImage() *common.ImageIdentifier {
	return a.PreviousImageData.Reference
}

func (a *analyzedMetadata) PreviousImageMetadata() common.LayersMetadata {
	return a.PreviousImageData.Metadata
}

func (a *analyzedMetadata) RunImage() *common.ImageIdentifier {
	return a.RunImageData.Reference
}

func (a *analyzedMetadata) RunImageMixins() []string {
	return a.RunImageData.Mixins
}
