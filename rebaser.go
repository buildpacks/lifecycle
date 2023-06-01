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
	var origMetadata files.LayersMetadataCompat
	if err := image.DecodeLabel(workingImage, platform.LayerMetadataLabel, &origMetadata); err != nil {
		return RebaseReport{}, errors.Wrap(err, "get image metadata")
	}

	appStackID, err := workingImage.Label(platform.StackIDLabel)
	if err != nil {
		return RebaseReport{}, errors.Wrap(err, "get app image stack")
	}

	newBaseStackID, err := newBaseImage.Label(platform.StackIDLabel)
	if err != nil {
		return RebaseReport{}, errors.Wrap(err, "get new base image stack")
	}

	if appStackID == "" {
		return RebaseReport{}, errors.New("stack not defined on app image")
	}

	if newBaseStackID == "" {
		return RebaseReport{}, errors.New("stack not defined on new base image")
	}

	if appStackID != newBaseStackID {
		return RebaseReport{}, fmt.Errorf("incompatible stack: '%s' is not compatible with '%s'", newBaseStackID, appStackID)
	}

	if err := r.validateRebaseable(workingImage, newBaseImage); err != nil {
		return RebaseReport{}, err
	}

	if err := validateMixins(workingImage, newBaseImage); err != nil {
		return RebaseReport{}, err
	}

	if err := workingImage.Rebase(origMetadata.RunImage.TopLayer, newBaseImage); err != nil {
		return RebaseReport{}, errors.Wrap(err, "rebase app image")
	}

	origMetadata.RunImage.TopLayer, err = newBaseImage.TopLayer()
	if err != nil {
		return RebaseReport{}, errors.Wrap(err, "get rebase run image top layer SHA")
	}

	identifier, err := newBaseImage.Identifier()
	if err != nil {
		return RebaseReport{}, errors.Wrap(err, "get run image id or digest")
	}
	origMetadata.RunImage.Reference = identifier.String()

	if r.PlatformAPI.AtLeast("0.12") {
		// update stack and runImage if needed
		if !origMetadata.RunImage.Contains(newBaseImage.Name()) {
			origMetadata.RunImage.Image = newBaseImage.Name()
			origMetadata.RunImage.Mirrors = []string{}
			newStackMD := origMetadata.RunImage.ToStack()
			origMetadata.Stack = &newStackMD
		}
	}

	data, err := json.Marshal(origMetadata)
	if err != nil {
		return RebaseReport{}, errors.Wrap(err, "marshall metadata")
	}

	if err := workingImage.SetLabel(platform.LayerMetadataLabel, string(data)); err != nil {
		return RebaseReport{}, errors.Wrap(err, "set app image metadata label")
	}

	hasPrefix := func(l string) bool {
		if r.PlatformAPI.AtLeast("0.12") {
			return strings.HasPrefix(l, "io.buildpacks.stack.") || strings.HasPrefix(l, "io.buildpacks.base.")
		}
		return strings.HasPrefix(l, "io.buildpacks.stack.")
	}
	if err := image.SyncLabels(newBaseImage, workingImage, hasPrefix); err != nil {
		return RebaseReport{}, errors.Wrap(err, "set stack labels")
	}
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

func (r *Rebaser) validateRebaseable(appImg imgutil.Image, newBaseImg imgutil.Image) error {
	if r.PlatformAPI.LessThan("0.12") {
		return nil
	}

	// skip validation if the previous image was built before 0.12
	appPlatformAPI, err := appImg.Env(platform.EnvPlatformAPI)
	if err != nil {
		return errors.Wrap(err, "get app image platform API")
	}

	// if the image doesn't have the platform API set, treat it as if it was built before 0.12 and skip additional validation
	if appPlatformAPI == "" || api.MustParse(appPlatformAPI).LessThan("0.12") {
		return nil
	}

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

		if !newBaseTarget.IsValidRebaseTargetFor(appTarget) {
			return fmt.Errorf("invalid base image target: '%s' is not equal to '%s'", newBaseTarget, appTarget)
		}
	}
	return nil
}

func (r *Rebaser) supportsManifestSize() bool {
	return r.PlatformAPI.AtLeast("0.6")
}
