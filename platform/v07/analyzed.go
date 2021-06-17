package v07

import (
	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/platform"
)

func (p *Platform) DecodeAnalyzedMetadataFile(path string) (platform.AnalyzedMetadata, error) {
	var (
		analyzedMd analyzedMetadata // TODO: change analyzedMD to analyzedMd
		err        error
	)

	if _, err = toml.DecodeFile(path, &analyzedMd); err == nil {
		return &analyzedMd, nil
	}
	return nil, err
}

func (p *Platform) NewAnalyzedMetadata(config platform.AnalyzedMetadataConfig) platform.AnalyzedMetadata {
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
	Reference *platform.ImageIdentifier `toml:"reference"`
	Metadata  platform.LayersMetadata   `toml:"metadata"`
}

type RunImage struct {
	Reference *platform.ImageIdentifier `toml:"reference"`
	Mixins    []string                  `toml:"mixins"`
}

func (a *analyzedMetadata) BuildImageStackID() string {
	return a.BuildImageData.StackID
}

func (a *analyzedMetadata) BuildImageMixins() []string {
	return a.BuildImageData.Mixins
}

func (a *analyzedMetadata) PreviousImage() *platform.ImageIdentifier {
	return a.PreviousImageData.Reference
}

func (a *analyzedMetadata) PreviousImageMetadata() platform.LayersMetadata {
	return a.PreviousImageData.Metadata
}

func (a *analyzedMetadata) RunImage() *platform.ImageIdentifier {
	return a.RunImageData.Reference
}

func (a *analyzedMetadata) RunImageMixins() []string {
	return a.RunImageData.Mixins
}
