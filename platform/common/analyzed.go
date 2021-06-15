package common

import (
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/buildpack/layertypes"
)

type AnalyzedMetadata interface {
	BuildImageStackID() string
	BuildImageMixins() []string
	PreviousImage() *ImageIdentifier
	PreviousImageMetadata() LayersMetadata
	RunImage() *ImageIdentifier
	RunImageMixins() []string
}

type AnalyzedMetadataBuilder interface {
	Build() AnalyzedMetadata

	WithBuildImageMixins(mixins []string) AnalyzedMetadataBuilder
	WithBuildImageStackID(stackID string) AnalyzedMetadataBuilder
	WithPreviousImage(imageID *ImageIdentifier) AnalyzedMetadataBuilder
	WithPreviousImageMetadata(meta LayersMetadata) AnalyzedMetadataBuilder
	WithRunImageMixins(mixins []string) AnalyzedMetadataBuilder
}

// analyzed.toml

// FIXME: fix key names to be accurate in the daemon case
type ImageIdentifier struct {
	Reference string `toml:"reference"`
}

// NOTE: This struct MUST be kept in sync with `LayersMetadataCompat`
type LayersMetadata struct {
	App          []LayerMetadata           `json:"app" toml:"app"`
	Buildpacks   []BuildpackLayersMetadata `json:"buildpacks" toml:"buildpacks"`
	Config       LayerMetadata             `json:"config" toml:"config"`
	Launcher     LayerMetadata             `json:"launcher" toml:"launcher"`
	ProcessTypes LayerMetadata             `json:"process-types" toml:"process-types"`
	RunImage     RunImageMetadata          `json:"runImage" toml:"run-image"`
	Stack        StackMetadata             `json:"stack" toml:"stack"`
}

// NOTE: This struct MUST be kept in sync with `LayersMetadata`.
// It exists for situations where the `App` field type cannot be
// guaranteed, yet the original struct data must be maintained.
type LayersMetadataCompat struct {
	App          interface{}               `json:"app" toml:"app"`
	Buildpacks   []BuildpackLayersMetadata `json:"buildpacks" toml:"buildpacks"`
	Config       LayerMetadata             `json:"config" toml:"config"`
	Launcher     LayerMetadata             `json:"launcher" toml:"launcher"`
	ProcessTypes LayerMetadata             `json:"process-types" toml:"process-types"`
	RunImage     RunImageMetadata          `json:"runImage" toml:"run-image"`
	Stack        StackMetadata             `json:"stack" toml:"stack"`
}

func (m *LayersMetadata) MetadataForBuildpack(id string) BuildpackLayersMetadata {
	for _, bpMD := range m.Buildpacks {
		if bpMD.ID == id {
			return bpMD
		}
	}
	return BuildpackLayersMetadata{}
}

type LayerMetadata struct {
	SHA string `json:"sha" toml:"sha"`
}

type BuildpackLayersMetadata struct {
	ID      string                            `json:"key" toml:"key"`
	Version string                            `json:"version" toml:"version"`
	Layers  map[string]BuildpackLayerMetadata `json:"layers" toml:"layers"`
	Store   *buildpack.StoreTOML              `json:"store,omitempty" toml:"store"`
}

type BuildpackLayerMetadata struct {
	LayerMetadata
	layertypes.LayerMetadataFile
}

type RunImageMetadata struct {
	TopLayer  string `json:"topLayer" toml:"top-layer"`
	Reference string `json:"reference" toml:"reference"`
}
