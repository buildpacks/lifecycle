package v06

import "github.com/buildpacks/lifecycle/platform/common"

func (p *Platform) DecodeAnalyzedMetadataFile(path string) (common.AnalyzedMetadata, error) {
	return p.previousPlatform.DecodeAnalyzedMetadataFile(path)
}

func (p *Platform) NewAnalyzedMetadata(config common.AnalyzedMetadataConfig) common.AnalyzedMetadata {
	return p.previousPlatform.NewAnalyzedMetadata(config)
}
