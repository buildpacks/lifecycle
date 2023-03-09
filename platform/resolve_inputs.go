package platform

import (
	"errors"
	"os"

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

type AccessChecker interface {
	CheckReadAccess(imageRef string) (bool, error)
}

func ResolveInputs(phase LifecyclePhase, i *LifecycleInputs, accessChecker AccessChecker, logger log.Logger) error {
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
		if err = op(i, accessChecker, logger); err != nil {
			return err
		}
	}
	return nil
}

// operations

type LifecycleInputsOperation func(i *LifecycleInputs, accessChecker AccessChecker, logger log.Logger) error

func CheckCache(i *LifecycleInputs, _ AccessChecker, logger log.Logger) error {
	if i.CacheImageRef == "" && i.CacheDir == "" {
		logger.Warn("No cached data will be used, no cache specified.")
	}
	return nil
}

func CheckLaunchCache(i *LifecycleInputs, _ AccessChecker, logger log.Logger) error {
	if !i.UseDaemon && i.LaunchCacheDir != "" {
		logger.Warn(MsgIgnoringLaunchCache)
	}
	return nil
}

func FillAnalyzeImages(i *LifecycleInputs, accessChecker AccessChecker, logger log.Logger) error {
	if i.PreviousImageRef == "" {
		i.PreviousImageRef = i.OutputImageRef
	}
	if i.PlatformAPI.LessThan("0.7") {
		return nil
	}
	if i.PlatformAPI.LessThan("0.12") {
		return fillRunImageFromStackTOMLIfNeeded(i, accessChecker, logger)
	}
	return fillRunImageFromRunTOMLIfNeeded(i, accessChecker, logger)
}

func FillCreateImages(i *LifecycleInputs, accessChecker AccessChecker, logger log.Logger) error {
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
		return fillRunImageFromStackTOMLIfNeeded(i, accessChecker, logger)
	default:
		return fillRunImageFromRunTOMLIfNeeded(i, accessChecker, logger)
	}
}

func FillExportRunImage(i *LifecycleInputs, accessChecker AccessChecker, logger log.Logger) error {
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
			return fillRunImageFromStackTOMLIfNeeded(i, accessChecker, logger)
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
			if analyzedMD.RunImage.Reference == "" {
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
func fillRunImageFromRunTOMLIfNeeded(i *LifecycleInputs, _ AccessChecker, logger log.Logger) error {
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
func fillRunImageFromStackTOMLIfNeeded(i *LifecycleInputs, _ AccessChecker, logger log.Logger) error {
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

// ValidateImageRefs ensures all provided image references are valid.
func ValidateImageRefs(i *LifecycleInputs, _ AccessChecker, _ log.Logger) error {
	for _, imageRef := range i.Images() {
		_, err := name.ParseReference(imageRef, name.WeakValidation)
		if err != nil {
			return err
		}
	}
	return nil
}

func ValidateOutputImageProvided(i *LifecycleInputs, _ AccessChecker, _ log.Logger) error {
	if i.OutputImageRef == "" {
		return errors.New(ErrOutputImageRequired)
	}
	return nil
}

func ValidateRebaseRunImage(i *LifecycleInputs, _ AccessChecker, _ log.Logger) error {
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
func ValidateTargetsAreSameRegistry(i *LifecycleInputs, _ AccessChecker, _ log.Logger) error {
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
