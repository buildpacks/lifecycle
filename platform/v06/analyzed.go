package v06

import "github.com/buildpacks/lifecycle/platform/common"

func (p *Platform) DecodeAnalyzedMetadata(contents string) (common.AnalyzedMetadata, error) {
	return p.previousPlatform.DecodeAnalyzedMetadata(contents)
}

func (p *Platform) DecodeAnalyzedMetadataFile(path string) (common.AnalyzedMetadata, error) {
	return p.previousPlatform.DecodeAnalyzedMetadataFile(path)
}

func (p *Platform) NewAnalyzedMetadataBuilder() common.AnalyzedMetadataBuilder {
	return p.previousPlatform.NewAnalyzedMetadataBuilder()
}
