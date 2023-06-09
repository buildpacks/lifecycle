package lifecycle

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
)

type Rebaser struct {
	Logger      log.Logger
	PlatformAPI *api.Version
	Force       bool
}

type RebaseReport struct {
	Image files.ImageReport `toml:"image"`
}

func (r *Rebaser) Rebase(workingImage imgutil.Image, newBaseImage imgutil.Image, outputImageRef string, additionalNames []string) (RebaseReport, error) {
	appPlatformAPI, err := workingImage.Env(platform.EnvPlatformAPI)
	if err != nil {
		return RebaseReport{}, fmt.Errorf("failed to get app image platform API: %w", err)
	}
	// perform platform API-specific validations
	if appPlatformAPI == "" || api.MustParse(appPlatformAPI).LessThan("0.12") {
		if err = validateStackID(workingImage, newBaseImage); err != nil {
			return RebaseReport{}, err
		}
		if err = validateMixins(workingImage, newBaseImage); err != nil {
			return RebaseReport{}, err
		}
	} else {
		if err = r.validateTarget(workingImage, newBaseImage); err != nil {
			return RebaseReport{}, err
		}
	}

	// get existing metadata label
	var origMetadata files.LayersMetadataCompat
	if err = image.DecodeLabel(workingImage, platform.LifecycleMetadataLabel, &origMetadata); err != nil {
		return RebaseReport{}, errors.Wrap(err, "get image metadata")
	}

	// rebase
	if err = workingImage.Rebase(origMetadata.RunImage.TopLayer, newBaseImage); err != nil {
		return RebaseReport{}, errors.Wrap(err, "rebase app image")
	}

	// update metadata label
	origMetadata.RunImage.TopLayer, err = newBaseImage.TopLayer()
	if err != nil {
		return RebaseReport{}, errors.Wrap(err, "get rebase run image top layer SHA")
	}
	identifier, err := newBaseImage.Identifier()
	if err != nil {
		return RebaseReport{}, errors.Wrap(err, "get run image id or digest")
	}
	origMetadata.RunImage.Reference = identifier.String()
	if appPlatformAPI != "" && api.MustParse(appPlatformAPI).AtLeast("0.12") {
		// update stack and runImage if needed
		if !origMetadata.RunImage.Contains(newBaseImage.Name()) {
			origMetadata.RunImage.Image = newBaseImage.Name()
			origMetadata.RunImage.Mirrors = []string{}
			newStackMD := origMetadata.RunImage.ToStack()
			origMetadata.Stack = &newStackMD
		}
	}

	// set metadata label
	data, err := json.Marshal(origMetadata)
	if err != nil {
		return RebaseReport{}, errors.Wrap(err, "marshall metadata")
	}
	if err := workingImage.SetLabel(platform.LifecycleMetadataLabel, string(data)); err != nil {
		return RebaseReport{}, errors.Wrap(err, "set app image metadata label")
	}

	// update other labels
	hasPrefix := func(l string) bool {
		if r.PlatformAPI.AtLeast("0.12") {
			return strings.HasPrefix(l, "io.buildpacks.stack.") || strings.HasPrefix(l, "io.buildpacks.base.")
		}
		return strings.HasPrefix(l, "io.buildpacks.stack.")
	}
	if err := image.SyncLabels(newBaseImage, workingImage, hasPrefix); err != nil {
		return RebaseReport{}, errors.Wrap(err, "set stack labels")
	}

	// save
	report := RebaseReport{}
	report.Image, err = saveImageAs(workingImage, outputImageRef, additionalNames, r.Logger)
	if err != nil {
		return RebaseReport{}, err
	}
	if !r.supportsManifestSize() {
		// unset manifest size in report.toml for old platform API versions
		report.Image.ManifestSize = 0
	}

	return report, err
}

func validateStackID(appImg, newBaseImage imgutil.Image) error {
	appStackID, err := appImg.Label(platform.StackIDLabel)
	if err != nil {
		return errors.Wrap(err, "get app image stack")
	}

	newBaseStackID, err := newBaseImage.Label(platform.StackIDLabel)
	if err != nil {
		return errors.Wrap(err, "get new base image stack")
	}

	if appStackID == "" {
		return errors.New("stack not defined on app image")
	}

	if newBaseStackID == "" {
		return errors.New("stack not defined on new base image")
	}

	if appStackID != newBaseStackID {
		return fmt.Errorf("incompatible stack: '%s' is not compatible with '%s'", newBaseStackID, appStackID)
	}
	return nil
}

func validateMixins(appImg, newBaseImg imgutil.Image) error {
	var appImageMixins []string
	var newBaseImageMixins []string

	if err := image.DecodeLabel(appImg, platform.MixinsLabel, &appImageMixins); err != nil {
		return errors.Wrap(err, "get app image mixins")
	}

	if err := image.DecodeLabel(newBaseImg, platform.MixinsLabel, &newBaseImageMixins); err != nil {
		return errors.Wrap(err, "get run image mixins")
	}

	appImageMixins = removeStagePrefixes(appImageMixins)
	newBaseImageMixins = removeStagePrefixes(newBaseImageMixins)

	_, missing, _ := str.Compare(newBaseImageMixins, appImageMixins)

	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing required mixin(s): %s", strings.Join(missing, ", "))
	}

	return nil
}

func (r *Rebaser) validateTarget(appImg imgutil.Image, newBaseImg imgutil.Image) error {
	rebaseable, err := appImg.Label(platform.RebaseableLabel)
	if err != nil {
		return errors.Wrap(err, "get app image rebaseable label")
	}
	if !r.Force && rebaseable == "false" {
		return fmt.Errorf("app image is not marked as rebaseable")
	}

	// check the OS, architecture, and variant values
	// if they are not the same, the image cannot be rebased unless the force flag is set
	if !r.Force {
		appTarget, err := platform.GetTargetMetadata(appImg)
		if err != nil {
			return errors.Wrap(err, "get app image target")
		}

		newBaseTarget, err := platform.GetTargetMetadata(newBaseImg)
		if err != nil {
			return errors.Wrap(err, "get new base image target")
		}

		if !platform.TargetSatisfiedForRebase(*newBaseTarget, *appTarget) {
			return fmt.Errorf(
				"unable to satisfy target os/arch constraints; new run image: %s, old run image: %s",
				encoding.ToJSONMaybe(newBaseTarget),
				encoding.ToJSONMaybe(appTarget),
			)
		}
	}
	return nil
}

func (r *Rebaser) supportsManifestSize() bool {
	return r.PlatformAPI.AtLeast("0.6")
}
