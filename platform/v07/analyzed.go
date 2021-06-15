package v07

import "github.com/buildpacks/lifecycle/platform/common"

func (p *Platform) DecodeAnalyzedMetadata(path string) (common.AnalyzedMetadata, error) {
	return nil, nil // TODO
}

// AnalyzedMetadataBuilder

type analyzedMetadataBuilder struct {
	ops []analyzedMetadataOp
}

type analyzedMetadataOp func(analyzedMetadata)

func (a *analyzedMetadataBuilder) Build() common.AnalyzedMetadata {
	meta := analyzedMetadata{}
	for _, op := range a.ops {
		op(meta)
	}
	return &meta
}

func (a *analyzedMetadataBuilder) WithPreviousImage(imageID *common.ImageIdentifier) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD analyzedMetadata) {
		analyzedMD.PreviousImageData.Reference = imageID
	})
	return a
}

func (a *analyzedMetadataBuilder) WithPreviousImageMetadata(meta common.LayersMetadata) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD analyzedMetadata) {
		analyzedMD.PreviousImageData.Metadata = meta
	})
	return a
}

func (p *Platform) NewAnalyzedMetadataBuilder() common.AnalyzedMetadataBuilder {
	return &analyzedMetadataBuilder{}
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
