package v06

import "github.com/buildpacks/lifecycle/platform/common"

func (p *Platform) DecodeAnalyzedMetadata(path string) (common.AnalyzedMetadata, error) {
	return p.previousPlatform.DecodeAnalyzedMetadata(path)
}

func (p *Platform) NewAnalyzedMetadataBuilder() common.AnalyzedMetadataBuilder {
	return p.previousPlatform.NewAnalyzedMetadataBuilder()
}
