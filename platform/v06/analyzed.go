package v06

import (
	"github.com/buildpacks/lifecycle/platform"
)

func (p *Platform) DecodeAnalyzedMetadataFile(path string) (platform.AnalyzedMetadata, error) {
	return p.previousPlatform.DecodeAnalyzedMetadataFile(path)
}

func (p *Platform) NewAnalyzedMetadata(config platform.AnalyzedMetadataConfig) platform.AnalyzedMetadata {
	return p.previousPlatform.NewAnalyzedMetadata(config)
}
