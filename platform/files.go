// Data Format Files for the Platform API spec (https://github.com/buildpacks/spec/blob/main/platform.md#data-format).

package platform

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/buildpacks/lifecycle/internal/fsutil"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/imgutil/remote"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/log"
)

// analyzed.toml

type AnalyzedMetadata struct {
	PreviousImage *ImageIdentifier `toml:"image,omitempty"`
	Metadata      LayersMetadata   `toml:"metadata"`
	RunImage      *RunImage        `toml:"run-image,omitempty"`
	BuildImage    *ImageIdentifier `toml:"build-image,omitempty"`
}

func (amd AnalyzedMetadata) PreviousImageRef() string {
	if amd.PreviousImage == nil {
		return ""
	}
	return amd.PreviousImage.Reference
}

func (amd AnalyzedMetadata) RunImageRef() string {
	if amd.RunImage == nil {
		return ""
	}
	return amd.RunImage.Reference
}

func (amd AnalyzedMetadata) RunImageTarget() TargetMetadata {
	if amd.RunImage == nil {
		return TargetMetadata{}
	}
	if amd.RunImage.TargetMetadata == nil {
		return TargetMetadata{}
	}
	return *amd.RunImage.TargetMetadata
}

// FIXME: fix key names to be accurate in the daemon case
type ImageIdentifier struct {
	Reference string `toml:"reference"`
}

type RunImage struct {
	Reference string `toml:"reference"`
	// Image specifies the repository name for the image.
	// When exporting to a daemon, the restorer uses this field to pull the run image if needed for the extender;
	// it can't use reference because this may be a daemon image ID if analyzed.toml was last written by the analyzer.
	Image          string          `toml:"image,omitempty"`
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

type OSDistribution struct {
	Name    string `json:"name" toml:"name"`
	Version string `json:"version" toml:"version"`
}

// IsSatisfiedBy treats optional fields (ArchVariant and Distributions) as wildcards if empty, returns true if
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

func (t *TargetMetadata) String() string {
	var distName, distVersion string
	if t.Distribution != nil {
		distName = t.Distribution.Name
		distVersion = t.Distribution.Version
	}
	return fmt.Sprintf("OS: %s, Arch: %s, ArchVariant: %s, Distribution: (Name: %s, Version: %s)", t.OS, t.Arch, t.ArchVariant, distName, distVersion)
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

func ReadAnalyzed(analyzedPath string, logger log.Logger) (AnalyzedMetadata, error) {
	var analyzedMD AnalyzedMetadata
	if _, err := toml.DecodeFile(analyzedPath, &analyzedMD); err != nil {
		if os.IsNotExist(err) {
			logger.Warnf("no analyzed metadata found at path '%s'", analyzedPath)
			return AnalyzedMetadata{}, nil
		}
		return AnalyzedMetadata{}, err
	}
	return analyzedMD, nil
}

// WriteTOML serializes the metadata to disk
func (amd *AnalyzedMetadata) WriteTOML(path string) error {
	return encoding.WriteTOML(path, amd)
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
	Stack        StackMetadata              `json:"stack,omitempty" toml:"stack,omitempty"`
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
	Stack        StackMetadata              `json:"stack" toml:"stack"`
}

func (m *LayersMetadata) MetadataForBuildpack(id string) buildpack.LayersMetadata {
	for _, bpMD := range m.Buildpacks {
		if bpMD.ID == id {
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

// metadata.toml

type BuildMetadata struct {
	BOM                         []buildpack.BOMEntry     `toml:"bom,omitempty" json:"bom"`
	Buildpacks                  []buildpack.GroupElement `toml:"buildpacks" json:"buildpacks"`
	Extensions                  []buildpack.GroupElement `toml:"extensions,omitempty" json:"extensions,omitempty"`
	Labels                      []buildpack.Label        `toml:"labels" json:"-"`
	Launcher                    LauncherMetadata         `toml:"-" json:"launcher"`
	Processes                   []launch.Process         `toml:"processes" json:"processes"`
	Slices                      []layers.Slice           `toml:"slices" json:"-"`
	BuildpackDefaultProcessType string                   `toml:"buildpack-default-process-type,omitempty" json:"buildpack-default-process-type,omitempty"`
	PlatformAPI                 *api.Version             `toml:"-" json:"-"`
}

// DecodeBuildMetadataTOML reads a metadata.toml file
func DecodeBuildMetadataTOML(path string, platformAPI *api.Version, buildmd *BuildMetadata) error {
	// decode the common bits
	_, err := toml.DecodeFile(path, &buildmd)
	if err != nil {
		return err
	}

	// set the platform API on all the appropriate fields
	// this will allow us to re-encode the metadata.toml file with
	// the current platform API
	buildmd.PlatformAPI = platformAPI
	for i, process := range buildmd.Processes {
		buildmd.Processes[i] = process.WithPlatformAPI(platformAPI)
	}

	return nil
}

func (md *BuildMetadata) MarshalJSON() ([]byte, error) {
	if md.PlatformAPI == nil || md.PlatformAPI.LessThan("0.9") {
		return json.Marshal(*md)
	}
	type BuildMetadataSerializer BuildMetadata // prevent infinite recursion when serializing
	return json.Marshal(&struct {
		*BuildMetadataSerializer
		BOM []buildpack.BOMEntry `json:"bom,omitempty"`
	}{
		BuildMetadataSerializer: (*BuildMetadataSerializer)(md),
		BOM:                     []buildpack.BOMEntry{},
	})
}

func (md BuildMetadata) ToLaunchMD() launch.Metadata {
	lmd := launch.Metadata{
		Processes: md.Processes,
	}
	for _, bp := range md.Buildpacks {
		lmd.Buildpacks = append(lmd.Buildpacks, launch.Buildpack{
			API: bp.API,
			ID:  bp.ID,
		})
	}
	return lmd
}

type LauncherMetadata struct {
	Version string         `json:"version"`
	Source  SourceMetadata `json:"source"`
}

type SourceMetadata struct {
	Git GitMetadata `json:"git"`
}

type GitMetadata struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

// plan.toml

type BuildPlan struct {
	Entries []BuildPlanEntry `toml:"entries"`
}

func (p BuildPlan) Find(kind, id string) buildpack.Plan {
	var extension bool
	if kind == buildpack.KindExtension {
		extension = true
	}
	var out []buildpack.Require
	for _, entry := range p.Entries {
		for _, provider := range entry.Providers {
			if provider.ID == id && provider.Extension == extension {
				out = append(out, entry.Requires...)
				break
			}
		}
	}
	return buildpack.Plan{Entries: out}
}

// FIXME: ensure at least one claimed entry of each name is provided by the BP
func (p BuildPlan) Filter(metRequires []string) BuildPlan {
	var out []BuildPlanEntry
	for _, planEntry := range p.Entries {
		if !containsEntry(metRequires, planEntry) {
			out = append(out, planEntry)
		}
	}
	return BuildPlan{Entries: out}
}

func containsEntry(metRequires []string, entry BuildPlanEntry) bool {
	for _, met := range metRequires {
		for _, planReq := range entry.Requires {
			if met == planReq.Name {
				return true
			}
		}
	}
	return false
}

type BuildPlanEntry struct {
	Providers []buildpack.GroupElement `toml:"providers"`
	Requires  []buildpack.Require      `toml:"requires"`
}

func (be BuildPlanEntry) NoOpt() BuildPlanEntry {
	var out []buildpack.GroupElement
	for _, p := range be.Providers {
		out = append(out, p.NoOpt().NoAPI().NoHomepage())
	}
	be.Providers = out
	return be
}

// project-metadata.toml

type ProjectMetadata struct {
	Source *ProjectSource `toml:"source" json:"source,omitempty"`
}

type ProjectSource struct {
	Type     string                 `toml:"type" json:"type,omitempty"`
	Version  map[string]interface{} `toml:"version" json:"version,omitempty"`
	Metadata map[string]interface{} `toml:"metadata" json:"metadata,omitempty"`
}

// report.toml

type ExportReport struct {
	Build BuildReport `toml:"build,omitempty"`
	Image ImageReport `toml:"image"`
}

type BuildReport struct {
	BOM []buildpack.BOMEntry `toml:"bom"`
}

type ImageReport struct {
	Tags         []string `toml:"tags"`
	ImageID      string   `toml:"image-id,omitempty"`
	Digest       string   `toml:"digest,omitempty"`
	ManifestSize int64    `toml:"manifest-size,omitzero"`
}

// run.toml

type RunMetadata struct {
	Images []RunImageForExport `json:"-" toml:"images"`
}

func ReadRun(runPath string, logger log.Logger) (RunMetadata, error) {
	var runMD RunMetadata
	if _, err := toml.DecodeFile(runPath, &runMD); err != nil {
		if os.IsNotExist(err) {
			logger.Infof("no run metadata found at path '%s'\n", runPath)
			return RunMetadata{}, nil
		}
		return RunMetadata{}, err
	}
	return runMD, nil
}

// stack.toml

type StackMetadata struct {
	RunImage RunImageForExport `json:"runImage" toml:"run-image"`
}

type RunImageForExport struct {
	Image   string   `toml:"image" json:"image"`
	Mirrors []string `toml:"mirrors" json:"mirrors,omitempty"`
}

type ImageStrategy interface {
	CheckReadAccess(repo string, keychain authn.Keychain) (bool, error)
}

type RemoteImageStrategy struct{}

var _ ImageStrategy = &RemoteImageStrategy{}

func (a *RemoteImageStrategy) CheckReadAccess(repo string, keychain authn.Keychain) (bool, error) {
	img, err := remote.NewImage(repo, keychain)
	if err != nil {
		return false, errors.Wrap(err, "failed to parse image reference")
	}

	return img.CheckReadAccess(), nil
}

type LocalImageStrategy struct{}

var _ ImageStrategy = &LocalImageStrategy{}

func (a *LocalImageStrategy) CheckReadAccess(_ string, _ authn.Keychain) (bool, error) {
	return true, nil
}

func (rm *RunImageForExport) BestRunImageMirror(registry string, accessChecker ImageStrategy) (string, error) {
	if rm.Image == "" {
		return "", errors.New("missing run-image metadata")
	}

	runImageMirrors := []string{rm.Image}
	runImageMirrors = append(runImageMirrors, rm.Mirrors...)

	keychain, err := auth.DefaultKeychain(runImageMirrors...)
	if err != nil {
		return "", errors.Wrap(err, "unable to create keychain")
	}

	runImageRef := byRegistry(registry, runImageMirrors, accessChecker, keychain)
	if runImageRef != "" {
		return runImageRef, nil
	}

	for _, image := range runImageMirrors {
		ok, err := accessChecker.CheckReadAccess(image, keychain)
		if err != nil {
			return "", err
		}

		if ok {
			return image, nil
		}
	}

	return "", errors.New("failed to find accessible run image")
}

func (sm *StackMetadata) BestRunImageMirror(registry string, accessChecker ImageStrategy) (string, error) {
	return sm.RunImage.BestRunImageMirror(registry, accessChecker)
}

func byRegistry(reg string, repos []string, accessChecker ImageStrategy, keychain authn.Keychain) string {
	for _, repo := range repos {
		ref, err := name.ParseReference(repo, name.WeakValidation)
		if err != nil {
			continue
		}
		if reg == ref.Context().RegistryStr() {
			ok, err := accessChecker.CheckReadAccess(repo, keychain)
			if err != nil {
				return ""
			}

			if ok {
				return repo
			}
		}
	}

	return ""
}

func ReadStack(stackPath string, logger log.Logger) (StackMetadata, error) {
	var stackMD StackMetadata
	if _, err := toml.DecodeFile(stackPath, &stackMD); err != nil {
		if os.IsNotExist(err) {
			logger.Infof("no stack metadata found at path '%s'\n", stackPath)
			return StackMetadata{}, nil
		}
		return StackMetadata{}, err
	}
	return stackMD, nil
}
