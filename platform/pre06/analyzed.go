package pre06

import "github.com/buildpacks/lifecycle/platform/common"

func (p *Platform) DecodeAnalyzedMetadata(contents string) (common.AnalyzedMetadata, error) {
	return nil, nil // TODO
}

func (p *Platform) DecodeAnalyzedMetadataFile(path string) (common.AnalyzedMetadata, error) {
	return nil, nil // TODO
}

// AnalyzedMetadataBuilder

type analyzedMetadataBuilder struct {
	ops []analyzedMetadataOp
}

func (a *analyzedMetadataBuilder) Build() common.AnalyzedMetadata {
	meta := analyzedMetadata{}
	for _, op := range a.ops {
		op(meta)
	}
	return &meta
}

func (a *analyzedMetadataBuilder) WithBuildImageMixins(mixins []string) common.AnalyzedMetadataBuilder {
	return a // nop
}

func (a *analyzedMetadataBuilder) WithBuildImageStackID(stackID string) common.AnalyzedMetadataBuilder {
	return a // nop
}

func (a *analyzedMetadataBuilder) WithPreviousImage(imageID *common.ImageIdentifier) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD analyzedMetadata) {
		analyzedMD.Image = imageID
	})
	return a
}

func (a *analyzedMetadataBuilder) WithPreviousImageMetadata(meta common.LayersMetadata) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD analyzedMetadata) {
		analyzedMD.Metadata = meta
	})
	return a
}

func (a *analyzedMetadataBuilder) WithRunImageMixins(mixins []string) common.AnalyzedMetadataBuilder {
	return a // nop
}

type analyzedMetadataOp func(analyzedMetadata)

func (p *Platform) NewAnalyzedMetadataBuilder() common.AnalyzedMetadataBuilder {
	return &analyzedMetadataBuilder{}
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
