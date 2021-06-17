package pre06

import (
	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/platform"
)

func (p *Platform) DecodeAnalyzedMetadataFile(path string) (platform.AnalyzedMetadata, error) {
	var (
		analyzedMd analyzedMetadata
		err        error
	)

	if _, err = toml.DecodeFile(path, &analyzedMd); err == nil {
		return &analyzedMd, nil
	}
	return nil, err
}

func (p *Platform) NewAnalyzedMetadata(config platform.AnalyzedMetadataConfig) platform.AnalyzedMetadata {
	return &analyzedMetadata{
		Image:    config.PreviousImage,
		Metadata: config.PreviousImageMetadata,
	}
}

// AnalyzedMetadata

type analyzedMetadata struct {
	Image    *platform.ImageIdentifier `toml:"image"`
	Metadata platform.LayersMetadata   `toml:"metadata"`
}

func (a *analyzedMetadata) BuildImageStackID() string {
	return ""
}

func (a *analyzedMetadata) BuildImageMixins() []string {
	return []string{}
}

func (a *analyzedMetadata) PreviousImage() *platform.ImageIdentifier {
	return a.Image
}

func (a *analyzedMetadata) PreviousImageMetadata() platform.LayersMetadata {
	return a.Metadata
}

func (a *analyzedMetadata) RunImage() *platform.ImageIdentifier {
	return nil
}

func (a *analyzedMetadata) RunImageMixins() []string {
	return []string{}
}
