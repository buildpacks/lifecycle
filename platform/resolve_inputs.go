package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/log"
)

var (
	ErrOutputImageRequired           = "image argument is required"
	ErrRunImageRequiredWhenNoStackMD = "-run-image is required when there is no stack metadata available"
	ErrRunImageRequiredWhenNoRunMD   = "-run-image is required when there is no run metadata available"
	ErrSupplyOnlyOneRunImage         = "supply only one of -run-image or (deprecated) -image"
	ErrRunImageUnsupported           = "-run-image is unsupported"
	ErrImageUnsupported              = "-image is unsupported"
	MsgIgnoringLaunchCache           = "Ignoring -launch-cache, only intended for use with -daemon"
)

func ResolveInputs(phase LifecyclePhase, i *LifecycleInputs, logger log.Logger) error {
	// order of operations is important
	ops := []LifecycleInputsOperation{UpdatePlaceholderPaths, ResolveAbsoluteDirPaths}
	switch phase {
	case Analyze:
		if i.PlatformAPI.LessThan("0.7") {
			ops = append(ops, CheckCache)
		}
		ops = append(ops,
			FillAnalyzeImages,
			ValidateOutputImageProvided,
			CheckLaunchCache,
			ValidateImageRefs,
			ValidateTargetsAreSameRegistry,
		)
	case Build:
		// nop
	case Create:
		ops = append(ops,
			FillCreateImages,
			ValidateOutputImageProvided,
			CheckCache,
			CheckLaunchCache,
			ValidateImageRefs,
			ValidateTargetsAreSameRegistry,
		)
	case Detect:
		// nop
	case Export:
		ops = append(ops,
			FillExportRunImage,
			ValidateOutputImageProvided,
			CheckCache,
			CheckLaunchCache,
			ValidateImageRefs,
			ValidateTargetsAreSameRegistry,
		)
	case Extend:
		// nop
	case Rebase:
		ops = append(ops,
			ValidateRebaseRunImage,
			ValidateOutputImageProvided,
			ValidateImageRefs,
			ValidateTargetsAreSameRegistry,
		)
	case Restore:
		ops = append(ops, CheckCache)
	}

	var err error
	for _, op := range ops {
		if err = op(i, logger); err != nil {
			return err
		}
	}
	return nil
}

// operations

type LifecycleInputsOperation func(i *LifecycleInputs, logger log.Logger) error

func CheckCache(i *LifecycleInputs, logger log.Logger) error {
	if i.CacheImageRef == "" && i.CacheDir == "" {
		logger.Warn("No cached data will be used, no cache specified.")
	}
	return nil
}

func CheckLaunchCache(i *LifecycleInputs, logger log.Logger) error {
	if !i.UseDaemon && i.LaunchCacheDir != "" {
		logger.Warn(MsgIgnoringLaunchCache)
	}
	return nil
}

func FillAnalyzeImages(i *LifecycleInputs, logger log.Logger) error {
	if i.PreviousImageRef == "" {
		i.PreviousImageRef = i.OutputImageRef
	}
	if i.PlatformAPI.LessThan("0.7") {
		return nil
	}
	if i.PlatformAPI.LessThan("0.12") {
		return fillRunImageFromStackTOMLIfNeeded(i, logger)
	}
	return fillRunImageFromRunTOMLIfNeeded(i, logger)
}

func FillCreateImages(i *LifecycleInputs, logger log.Logger) error {
	if i.PreviousImageRef == "" {
		i.PreviousImageRef = i.OutputImageRef
	}
	switch {
	case i.DeprecatedRunImageRef != "" && i.RunImageRef != os.Getenv(EnvRunImage):
		return errors.New(ErrSupplyOnlyOneRunImage)
	case i.DeprecatedRunImageRef != "":
		i.RunImageRef = i.DeprecatedRunImageRef
		return nil
	case i.PlatformAPI.LessThan("0.12"):
		return fillRunImageFromStackTOMLIfNeeded(i, logger)
	default:
		return fillRunImageFromRunTOMLIfNeeded(i, logger)
	}
}

func FillExportRunImage(i *LifecycleInputs, logger log.Logger) error {
	supportsRunImageFlag := i.PlatformAPI.LessThan("0.7")
	if supportsRunImageFlag {
		switch {
		case i.DeprecatedRunImageRef != "" && i.RunImageRef != os.Getenv(EnvRunImage):
			return errors.New(ErrSupplyOnlyOneRunImage)
		case i.RunImageRef != "":
			return nil
		case i.DeprecatedRunImageRef != "":
			i.RunImageRef = i.DeprecatedRunImageRef
			return nil
		default:
			return fillRunImageFromStackTOMLIfNeeded(i, logger)
		}
	} else {
		switch {
		case i.RunImageRef != "" && i.RunImageRef != os.Getenv(EnvRunImage):
			return errors.New(ErrRunImageUnsupported)
		case i.DeprecatedRunImageRef != "":
			return errors.New(ErrImageUnsupported)
		default:
			analyzedMD, err := ReadAnalyzed(i.AnalyzedPath, logger)
			if err != nil {
				return err
			}
			if analyzedMD.RunImage == nil || analyzedMD.RunImage.Reference == "" {
				return errors.New("run image not found in analyzed metadata")
			}
			i.RunImageRef = analyzedMD.RunImage.Reference
			return nil
		}
	}
}

// fillRunImageFromRunTOMLIfNeeded updates the provided lifecycle inputs to include the run image from run.toml if the run image input it is missing.
// When there are multiple images in run.toml, the first image is selected.
// When there are registry mirrors for the selected image, the image with registry matching the output image is selected.
func fillRunImageFromRunTOMLIfNeeded(i *LifecycleInputs, logger log.Logger) error {
	if i.RunImageRef != "" {
		return nil
	}
	targetRegistry, err := parseRegistry(i.OutputImageRef)
	if err != nil {
		return err
	}
	runMD, err := ReadRun(i.RunPath, logger)
	if err != nil {
		return err
	}
	if len(runMD.Images) == 0 {
		return errors.New(ErrRunImageRequiredWhenNoRunMD)
	}
	i.RunImageRef, err = runMD.Images[0].BestRunImageMirror(targetRegistry)
	if err != nil {
		return errors.New(ErrRunImageRequiredWhenNoRunMD)
	}
	return nil
}

// fillRunImageFromStackTOMLIfNeeded updates the provided lifecycle inputs to include the run image from stack.toml if the run image input it is missing.
// When there are registry mirrors in stack.toml, the image with registry matching the output image is selected.
func fillRunImageFromStackTOMLIfNeeded(i *LifecycleInputs, logger log.Logger) error {
	if i.RunImageRef != "" {
		return nil
	}
	targetRegistry, err := parseRegistry(i.OutputImageRef)
	if err != nil {
		return err
	}
	stackMD, err := ReadStack(i.StackPath, logger)
	if err != nil {
		return err
	}
	i.RunImageRef, err = stackMD.BestRunImageMirror(targetRegistry)
	if err != nil {
		return errors.New(ErrRunImageRequiredWhenNoStackMD)
	}
	return nil
}

func parseRegistry(providedRef string) (string, error) {
	ref, err := name.ParseReference(providedRef, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}

func ResolveAbsoluteDirPaths(i *LifecycleInputs, _ log.Logger) error {
	toUpdate := i.directoryPaths()
	for _, dir := range toUpdate {
		if *dir == "" {
			continue
		}
		abs, err := filepath.Abs(*dir)
		if err != nil {
			return err
		}
		*dir = abs
	}
	return nil
}

func (i *LifecycleInputs) directoryPaths() []*string {
	return []*string{
		&i.AppDir,
		&i.BuildConfigDir,
		&i.BuildpacksDir,
		&i.CacheDir,
		&i.ExtensionsDir,
		&i.GeneratedDir,
		&i.KanikoDir,
		&i.LaunchCacheDir,
		&i.LayersDir,
		&i.PlatformDir,
	}
}

const placeholderLayersDir = "<layers>"

var (
	placeholderAnalyzedPath        = filepath.Join(placeholderLayersDir, DefaultAnalyzedFile)
	placeholderExtendedDir         = filepath.Join(placeholderLayersDir, DefaultExtendedDir)
	placeholderGeneratedDir        = filepath.Join(placeholderLayersDir, DefaultGeneratedDir)
	placeholderGroupPath           = filepath.Join(placeholderLayersDir, DefaultGroupFile)
	placeholderOrderPath           = filepath.Join(placeholderLayersDir, DefaultOrderFile)
	placeholderPlanPath            = filepath.Join(placeholderLayersDir, DefaultPlanFile)
	placeholderProjectMetadataPath = filepath.Join(placeholderLayersDir, DefaultProjectMetadataFile)
	placeholderReportPath          = filepath.Join(placeholderLayersDir, DefaultReportFile)
)

func UpdatePlaceholderPaths(i *LifecycleInputs, _ log.Logger) error {
	toUpdate := i.placeholderPaths()
	for _, pp := range toUpdate {
		switch {
		case *pp == "":
			continue
		case *pp == placeholderOrderPath:
			*pp = i.defaultOrderPath()
		case strings.Contains(*pp, placeholderLayersDir):
			filename := filepath.Base(*pp)
			*pp = filepath.Join(i.configDir(), filename)
		default:
			// nop
		}
	}
	return nil
}

// ValidateImageRefs ensures all provided image references are valid.
func ValidateImageRefs(i *LifecycleInputs, _ log.Logger) error {
	for _, imageRef := range i.Images() {
		_, err := name.ParseReference(imageRef, name.WeakValidation)
		if err != nil {
			return err
		}
	}
	return nil
}

func ValidateOutputImageProvided(i *LifecycleInputs, _ log.Logger) error {
	if i.OutputImageRef == "" {
		return errors.New(ErrOutputImageRequired)
	}
	return nil
}

func ValidateRebaseRunImage(i *LifecycleInputs, _ log.Logger) error {
	switch {
	case i.DeprecatedRunImageRef != "" && i.RunImageRef != os.Getenv(EnvRunImage):
		return errors.New(ErrSupplyOnlyOneRunImage)
	case i.DeprecatedRunImageRef != "":
		i.RunImageRef = i.DeprecatedRunImageRef
		return nil
	default:
		return nil
	}
}

// ValidateTargetsAreSameRegistry ensures all output images are on the same registry.
func ValidateTargetsAreSameRegistry(i *LifecycleInputs, _ log.Logger) error {
	if i.UseDaemon {
		return nil
	}
	return ValidateSameRegistry(i.DestinationImages()...)
}

func ValidateSameRegistry(tags ...string) error {
	var (
		reg        string
		registries = map[string]struct{}{}
	)
	for _, imageRef := range tags {
		ref, err := name.ParseReference(imageRef, name.WeakValidation)
		if err != nil {
			return err
		}
		reg = ref.Context().RegistryStr()
		registries[reg] = struct{}{}
	}

	if len(registries) > 1 {
		return errors.New("writing to multiple registries is unsupported")
	}
	return nil
}
