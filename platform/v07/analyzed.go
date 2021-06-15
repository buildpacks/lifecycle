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

// AnalyzedMetadataBuilder

type analyzedMetadataBuilder struct {
	ops []analyzedMetadataOp
}

func (a *analyzedMetadataBuilder) Build() common.AnalyzedMetadata {
	meta := analyzedMetadata{}
	for _, op := range a.ops {
		op(&meta)
	}
	return &meta
}

func (a *analyzedMetadataBuilder) WithBuildImageMixins(mixins []string) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD *analyzedMetadata) {
		analyzedMD.BuildImageData.Mixins = mixins
	})
	return a
}

func (a *analyzedMetadataBuilder) WithBuildImageStackID(stackID string) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD *analyzedMetadata) {
		analyzedMD.BuildImageData.StackID = stackID
	})
	return a
}

func (a *analyzedMetadataBuilder) WithPreviousImage(imageID *common.ImageIdentifier) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD *analyzedMetadata) {
		analyzedMD.PreviousImageData.Reference = imageID
	})
	return a
}

func (a *analyzedMetadataBuilder) WithPreviousImageMetadata(meta common.LayersMetadata) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD *analyzedMetadata) {
		analyzedMD.PreviousImageData.Metadata = meta
	})
	return a
}

func (a *analyzedMetadataBuilder) WithRunImageMixins(mixins []string) common.AnalyzedMetadataBuilder {
	a.ops = append(a.ops, func(analyzedMD *analyzedMetadata) {
		analyzedMD.RunImageData.Mixins = mixins
	})
	return a
}

type analyzedMetadataOp func(*analyzedMetadata)

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
