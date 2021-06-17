package pre06

import (
	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/platform/common"
)

func (p *Platform) DecodeAnalyzedMetadataFile(path string) (common.AnalyzedMetadata, error) {
	var (
		analyzedMd analyzedMetadata
		err        error
	)

	if _, err = toml.DecodeFile(path, &analyzedMd); err == nil {
		return &analyzedMd, nil
	}
	return nil, err
}

func (p *Platform) NewAnalyzedMetadata(config common.AnalyzedMetadataConfig) common.AnalyzedMetadata {
	return &analyzedMetadata{
		Image:    config.PreviousImage,
		Metadata: config.PreviousImageMetadata,
	}
}

// AnalyzedMetadata

type analyzedMetadata struct {
	Image    *common.ImageIdentifier `toml:"image"`
	Metadata common.LayersMetadata   `toml:"metadata"`
}

func (a *analyzedMetadata) BuildImageStackID() string {
	return ""
}

func (a *analyzedMetadata) BuildImageMixins() []string {
	return []string{}
}

func (a *analyzedMetadata) PreviousImage() *common.ImageIdentifier {
	return a.Image
}

func (a *analyzedMetadata) PreviousImageMetadata() common.LayersMetadata {
	return a.Metadata
}

func (a *analyzedMetadata) RunImage() *common.ImageIdentifier {
	return nil
}

func (a *analyzedMetadata) RunImageMixins() []string {
	return []string{}
}
