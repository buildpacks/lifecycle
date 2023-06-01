package files

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/log"
)

// Analyzed is written by the analyzer as analyzed.toml to record information about:
// * the previous image (if it exists),
// * the run image,
// * the build image (if provided).
type Analyzed struct {
	// PreviousImage is the build image identifier, if the previous image exists.
	PreviousImage *ImageIdentifier `toml:"image,omitempty"`
	// BuildImage is the build image identifier.
	// It is recorded for use by the restorer in the case that image extensions are used
	// to extend the build image.
	BuildImage *ImageIdentifier `toml:"build-image,omitempty"`
	// LayersMetadata holds information about previously built layers.
	// It is used by the exporter to determine if any layers from the current build are unchanged,
	// to avoid re-uploading the same data to the export target,
	// and to provide information about previously-created layers to buildpacks.
	LayersMetadata LayersMetadata `toml:"metadata"`
	// RunImage holds information about the run image.
	// It is used to validate that buildpacks satisfy os/arch constraints,
	// and to provide information about the export target to buildpacks.
	RunImage *RunImage `toml:"run-image,omitempty"`
}

func ReadAnalyzed(path string, logger log.Logger) (Analyzed, error) {
	var analyzed Analyzed
	if _, err := toml.DecodeFile(path, &analyzed); err != nil {
		if os.IsNotExist(err) {
			logger.Warnf("no analyzed metadata found at path '%s'", path)
			return Analyzed{}, nil
		}
		return Analyzed{}, err
	}
	return analyzed, nil
}

func (a Analyzed) PreviousImageRef() string {
	if a.PreviousImage == nil {
		return ""
	}
	return a.PreviousImage.Reference
}

func (a Analyzed) RunImageImage() string {
	if a.RunImage == nil {
		return ""
	}
	return a.RunImage.Image
}

func (a Analyzed) RunImageRef() string {
	if a.RunImage == nil {
		return ""
	}
	return a.RunImage.Reference
}

func (a Analyzed) RunImageTarget() TargetMetadata {
	if a.RunImage == nil {
		return TargetMetadata{}
	}
	if a.RunImage.TargetMetadata == nil {
		return TargetMetadata{}
	}
	return *a.RunImage.TargetMetadata
}

type ImageIdentifier struct {
	Reference string `toml:"reference"` // FIXME: fix key name to be accurate in the daemon case
}

// NOTE: This struct MUST be kept in sync with `LayersMetadataCompat`
type LayersMetadata struct {
	App          []LayerMetadata            `json:"app" toml:"app"`
	BOM          *LayerMetadata             `json:"sbom,omitempty" toml:"sbom,omitempty"`
	Buildpacks   []buildpack.LayersMetadata `json:"buildpacks" toml:"buildpacks"`
	Config       LayerMetadata              `json:"config" toml:"config"`
	Launcher     LayerMetadata              `json:"launcher" toml:"launcher"`
	ProcessTypes LayerMetadata              `json:"process-types" toml:"process-types"`
	RunImage     RunImageForRebase          `json:"runImage" toml:"run-image"`
	Stack        *Stack                     `json:"stack,omitempty" toml:"stack,omitempty"`
}

// NOTE: This struct MUST be kept in sync with `LayersMetadata`.
// It exists for situations where the `App` field type cannot be
// guaranteed, yet the original struct data must be maintained.
type LayersMetadataCompat struct {
	App          interface{}                `json:"app" toml:"app"`
	BOM          *LayerMetadata             `json:"sbom,omitempty" toml:"sbom,omitempty"`
	Buildpacks   []buildpack.LayersMetadata `json:"buildpacks" toml:"buildpacks"`
	Config       LayerMetadata              `json:"config" toml:"config"`
	Launcher     LayerMetadata              `json:"launcher" toml:"launcher"`
	ProcessTypes LayerMetadata              `json:"process-types" toml:"process-types"`
	RunImage     RunImageForRebase          `json:"runImage" toml:"run-image"`
	Stack        *Stack                     `json:"stack,omitempty" toml:"stack,omitempty"`
}

func (m *LayersMetadata) LayersMetadataFor(bpID string) buildpack.LayersMetadata {
	for _, bpMD := range m.Buildpacks {
		if bpMD.ID == bpID {
			return bpMD
		}
	}
	return buildpack.LayersMetadata{}
}

type LayerMetadata struct {
	SHA string `json:"sha" toml:"sha"`
}

type RunImageForRebase struct {
	TopLayer  string `json:"topLayer" toml:"top-layer"`
	Reference string `json:"reference" toml:"reference"`

	// added in Platform 0.12
	Image   string   `toml:"image,omitempty" json:"image,omitempty"`
	Mirrors []string `toml:"mirrors,omitempty" json:"mirrors,omitempty"`
}

func (r *RunImageForRebase) Contains(ref string) bool {
	ref = parseMaybe(ref)
	if parseMaybe(r.Image) == ref {
		return true
	}
	for _, m := range r.Mirrors {
		if parseMaybe(m) == ref {
			return true
		}
	}
	return false
}

func parseMaybe(ref string) string {
	if nameRef, err := name.ParseReference(ref); err == nil {
		return nameRef.Context().Name()
	}
	return ref
}

func (r *RunImageForRebase) ToStack() Stack {
	return Stack{
		RunImage: RunImageForExport{
			Image:   r.Image,
			Mirrors: r.Mirrors,
		},
	}
}

type RunImage struct {
	Reference string `toml:"reference"`
	// Image specifies the repository name for the image.
	// When exporting to a daemon, the restorer uses this field to pull the run image if needed for the extender;
	// it can't use reference because this may be a daemon image ID if analyzed.toml was last written by the analyzer.
	Image string `toml:"image,omitempty"`
	// Extend if true indicates that the run image should be extended by the extender.
	Extend         bool            `toml:"extend,omitempty"`
	TargetMetadata *TargetMetadata `json:"target,omitempty" toml:"target,omitempty"`
}

type TargetMetadata struct {
	ID          string `json:"id" toml:"id"`
	OS          string `json:"os" toml:"os"`
	Arch        string `json:"arch" toml:"arch"`
	ArchVariant string `json:"arch-variant" toml:"arch-variant"`

	Distribution *OSDistribution `json:"distribution,omitempty" toml:"distribution,omitempty"`
}

func (t *TargetMetadata) String() string {
	if t.Distribution != nil {
		return fmt.Sprintf("OS: %s, Arch: %s, ArchVariant: %s, Distribution: (Name: %s, Version: %s)", t.OS, t.Arch, t.ArchVariant, t.Distribution.Name, t.Distribution.Version)
	}
	return fmt.Sprintf("OS: %s, Arch: %s, ArchVariant: %s", t.OS, t.Arch, t.ArchVariant)
}

type OSDistribution struct {
	Name    string `json:"name" toml:"name"`
	Version string `json:"version" toml:"version"`
}

// FIXME: the target logic in this file might be better located in a dedicated package.

// IsSatisfiedBy treats optional fields (ArchVariant and Distributions) as wildcards if empty, returns true if all populated fields match
func (t *TargetMetadata) IsSatisfiedBy(o *buildpack.TargetMetadata) bool {
	if (o.Arch != "*" && t.Arch != o.Arch) || (o.OS != "*" && t.OS != o.OS) {
		return false
	}
	if t.ArchVariant != "" && o.ArchVariant != "" && t.ArchVariant != o.ArchVariant {
		return false
	}

	// if either of the lengths of Distributions are zero, treat it as a wildcard.
	if t.Distribution != nil && len(o.Distributions) > 0 {
		// this could be more efficient but the lists are probably short...
		found := false
		for _, odist := range o.Distributions {
			if t.Distribution.Name == odist.Name && t.Distribution.Version == odist.Version {
				found = true
				continue
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// IsValidRebaseTargetFor treats optional fields (ArchVariant and Distribution fields) as wildcards if empty, returns true if all populated fields match
func (t *TargetMetadata) IsValidRebaseTargetFor(appTargetMetadata *TargetMetadata) bool {
	if t.Arch != appTargetMetadata.Arch || t.OS != appTargetMetadata.OS {
		return false
	}
	if t.ArchVariant != "" && appTargetMetadata.ArchVariant != "" && t.ArchVariant != appTargetMetadata.ArchVariant {
		return false
	}

	if t.Distribution != nil && appTargetMetadata.Distribution != nil {
		if t.Distribution.Name != appTargetMetadata.Distribution.Name {
			return false
		}
		if t.Distribution.Version != appTargetMetadata.Distribution.Version {
			return false
		}
	}
	return true
}

// PopulateTargetOSFromFileSystem populates the target metadata you pass in if the information is available
// returns a boolean indicating whether it populated any data.
func PopulateTargetOSFromFileSystem(d fsutil.Detector, tm *TargetMetadata, logger log.Logger) {
	if d.HasSystemdFile() {
		contents, err := d.ReadSystemdFile()
		if err != nil {
			logger.Warnf("Encountered error trying to read /etc/os-release file: %s", err.Error())
			return
		}
		info := d.GetInfo(contents)
		if info.Version != "" || info.Name != "" {
			tm.OS = "linux"
			tm.Distribution = &OSDistribution{Name: info.Name, Version: info.Version}
		}
	}
}
