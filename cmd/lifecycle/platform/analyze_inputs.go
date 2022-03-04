package platform

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/str"
)

// AnalyzeInputs holds the values of command-line flags and args.
// Fields are the cumulative total of inputs across all supported platform APIs.
type AnalyzeInputs struct {
	AnalyzedPath string
	StackPath    string
	UID          int
	GID          int
	UseDaemon    bool

	AnalyzerOpts
}

// AnalyzerOpts holds the inputs needed to construct a new lifecycle.Analyzer.
// Fields are the cumulative total of inputs across all supported platform APIs.
type AnalyzerOpts struct {
	AdditionalTags   str.Slice
	CacheImageRef    string
	LaunchCacheDir   string
	LayersDir        string
	LegacyCacheDir   string
	LegacyGroupPath  string
	OutputImageRef   string
	PreviousImageRef string
	RunImageRef      string
	SkipLayers       bool

	LegacyGroup buildpack.Group // for creator
}

func (a AnalyzeInputs) RegistryImages() []string {
	var images []string
	images = appendNotEmpty(images, a.CacheImageRef)
	if !a.UseDaemon {
		images = appendNotEmpty(images, a.PreviousImageRef, a.RunImageRef, a.OutputImageRef)
		images = appendNotEmpty(images, a.AdditionalTags...)
	}
	return images
}

type AnalyzeInputsResolver struct {
	PlatformAPI *api.Version
}

// Resolve accepts AnalyzeInputs with flags filled in, and args.
// It returns AnalyzeInputs with default values filled in, or an error if the provided inputs are not valid.
func (av *AnalyzeInputsResolver) Resolve(inputs AnalyzeInputs, cmdArgs []string, logger lifecycle.Logger) (AnalyzeInputs, error) {
	resolvedInputs := inputs

	nargs := len(cmdArgs)
	if nargs != 1 {
		return AnalyzeInputs{}, fmt.Errorf("failed to parse arguments: received %d arguments, but expected 1", nargs)
	}
	resolvedInputs.OutputImageRef = cmdArgs[0]

	if err := av.fillDefaults(&resolvedInputs, logger); err != nil {
		return AnalyzeInputs{}, err
	}

	if err := av.validate(resolvedInputs, logger); err != nil {
		return AnalyzeInputs{}, err
	}
	return resolvedInputs, nil
}

func (av *AnalyzeInputsResolver) fillDefaults(inputs *AnalyzeInputs, logger lifecycle.Logger) error {
	if inputs.AnalyzedPath == PlaceholderAnalyzedPath {
		inputs.AnalyzedPath = defaultPath(PlaceholderAnalyzedPath, inputs.LayersDir, av.PlatformAPI)
	}

	if inputs.LegacyGroupPath == PlaceholderGroupPath {
		inputs.LegacyGroupPath = defaultPath(PlaceholderGroupPath, inputs.LayersDir, av.PlatformAPI)
	}

	if inputs.PreviousImageRef == "" {
		inputs.PreviousImageRef = inputs.OutputImageRef
	}

	return av.fillRunImage(inputs, logger)
}

func (av *AnalyzeInputsResolver) fillRunImage(inputs *AnalyzeInputs, logger lifecycle.Logger) error {
	if av.PlatformAPI.LessThan("0.7") || inputs.RunImageRef != "" {
		return nil
	}

	targetRegistry, err := parseRegistry(inputs.OutputImageRef)
	if err != nil {
		return err
	}

	stackMD, err := readStack(inputs.StackPath, logger)
	if err != nil {
		return err
	}

	inputs.RunImageRef, err = stackMD.BestRunImageMirror(targetRegistry)
	if err != nil {
		return errors.New("-run-image is required when there is no stack metadata available")
	}
	return nil
}

func (av *AnalyzeInputsResolver) validate(inputs AnalyzeInputs, logger lifecycle.Logger) error {
	if inputs.OutputImageRef == "" {
		return errors.New("image argument is required")
	}

	if !inputs.UseDaemon {
		if err := ensureSameRegistry(inputs.PreviousImageRef, inputs.OutputImageRef); err != nil {
			return errors.Wrap(err, "ensuring previous image and exported image are on same registry")
		}

		if inputs.LaunchCacheDir != "" {
			logger.Warn("Ignoring -launch-cache, only intended for use with -daemon")
		}
	}

	if err := image.ValidateDestinationTags(inputs.UseDaemon, append(inputs.AdditionalTags, inputs.OutputImageRef)...); err != nil {
		return errors.Wrap(err, "validating image tag(s)")
	}

	if av.PlatformAPI.AtLeast("0.7") {
		return nil
	}

	if inputs.CacheImageRef == "" && inputs.LegacyCacheDir == "" {
		logger.Warn("Not restoring cached layer metadata, no cache flag specified.")
	}
	return nil
}
