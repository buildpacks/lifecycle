package main

import (
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	//flags: inputs
	analyzeArgs
	stackPath string
	uid, gid  int

	//flags: paths to write data
	analyzedPath string
}

type analyzeArgs struct {
	cacheImageRef    string
	layersDir        string
	outputImageRef   string
	previousImageRef string
	runImageRef      string
	useDaemon        bool

	additionalTags cmd.StringSlice
	docker         client.CommonAPIClient // construct if necessary before dropping privileges
	keychain       authn.Keychain
	platform       Platform
	platform06     analyzeArgsPlatform06
}

type analyzeArgsPlatform06 struct {
	cacheDir   string // not needed when run by creator
	groupPath  string // not needed when run by creator
	skipLayers bool
	cache      lifecycle.Cache
	group      buildpack.Group
}

func (a *analyzeCmd) DefineFlags() {
	cmd.FlagAnalyzedPath(&a.analyzedPath)
	cmd.FlagCacheImage(&a.cacheImageRef)
	cmd.FlagLayersDir(&a.layersDir)
	if a.platformAPIVersionGreaterThan06() {
		cmd.FlagPreviousImage(&a.previousImageRef)
		cmd.FlagRunImage(&a.runImageRef)
		cmd.FlagStackPath(&a.stackPath)
		cmd.FlagTags(&a.additionalTags)
	} else {
		cmd.FlagCacheDir(&a.platform06.cacheDir)
		cmd.FlagGroupPath(&a.platform06.groupPath)
		cmd.FlagSkipLayers(&a.platform06.skipLayers)
	}
	cmd.FlagUseDaemon(&a.useDaemon)
	cmd.FlagUID(&a.uid)
	cmd.FlagGID(&a.gid)
}

func (a *analyzeCmd) Args(nargs int, args []string) error {
	if nargs != 1 {
		return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeInvalidArgs, "parse arguments")
	}

	if args[0] == "" {
		return cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}
	a.outputImageRef = args[0]

	if a.restoresLayerMetadata() {
		if a.cacheImageRef == "" && a.platform06.cacheDir == "" {
			cmd.DefaultLogger.Warn("Not restoring cached layer metadata, no cache flag specified.")
		}
	}

	targetRegistry, err := parseRegistry(a.outputImageRef)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse target registry")
	}

	if a.previousImageRef == "" {
		a.previousImageRef = a.outputImageRef
	} else if !a.useDaemon {
		previousRegistry, err := parseRegistry(a.previousImageRef)
		if err != nil {
			return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse previous registry")
		}
		if previousRegistry != targetRegistry {
			err := fmt.Errorf("previous image is on a different registry %s from the exported image %s", previousRegistry, targetRegistry)
			return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "validate registry")
		}
	}

	if err := image.ValidateDestinationTags(a.useDaemon, append(a.additionalTags, a.outputImageRef)...); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "validate image tag(s)")
	}

	if a.analyzedPath == cmd.PlaceholderAnalyzedPath {
		a.analyzedPath = cmd.DefaultAnalyzedPath(a.platform.API().String(), a.layersDir)
	}

	if a.platform06.groupPath == cmd.PlaceholderGroupPath {
		a.platform06.groupPath = cmd.DefaultGroupPath(a.platform.API().String(), a.layersDir)
	}

	if a.supportsRunImage() {
		stackMD, err := readStack(a.stackPath)
		if err != nil {
			return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse stack metadata")
		}

		if err := a.validateRunImageInput(); err != nil {
			return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "validate run image input")
		}

		if err := a.populateRunImage(stackMD, targetRegistry); err != nil {
			return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "populate run image")
		}
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

func (a *analyzeCmd) Privileges() error {
	var err error
	a.keychain, err = auth.DefaultKeychain(a.registryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}

	if a.useDaemon {
		var err error
		a.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if a.platformAPIVersionGreaterThan06() {
		if err := image.VerifyRegistryAccess(a, a.keychain); err != nil {
			return cmd.FailErr(err)
		}
	}
	if err := priv.EnsureOwner(a.uid, a.gid, a.layersDir, a.platform06.cacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(a.uid, a.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", a.uid, a.gid))
	}
	return nil
}

func (aa *analyzeArgs) registryImages() []string {
	var registryImages []string
	registryImages = append(registryImages, aa.ReadableRegistryImages()...)
	return append(registryImages, aa.WriteableRegistryImages()...)
}

func (a *analyzeCmd) Exec() error {
	var (
		group      buildpack.Group
		err        error
		cacheStore lifecycle.Cache
	)
	if a.restoresLayerMetadata() {
		group, err = lifecycle.ReadGroup(a.platform06.groupPath)
		if err != nil {
			return cmd.FailErr(err, "read buildpack group")
		}
		if err := verifyBuildpackApis(group); err != nil {
			return err
		}
		cacheStore, err = initCache(a.cacheImageRef, a.platform06.cacheDir, a.keychain)
		if err != nil {
			return cmd.FailErr(err, "initialize cache")
		}
		a.platform06.group = group
		a.platform06.cache = cacheStore
	}

	analyzedMD, err := a.analyze()
	if err != nil {
		return err
	}

	if err := lifecycle.WriteTOML(a.analyzedPath, analyzedMD); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}

func (aa analyzeArgs) analyze() (platform.AnalyzedMetadata, error) {
	previousImage, err := aa.localOrRemote(aa.previousImageRef)
	if err != nil {
		return platform.AnalyzedMetadata{}, cmd.FailErr(err, "get previous image")
	}

	runImage, err := aa.localOrRemote(aa.runImageRef)
	if err != nil {
		return platform.AnalyzedMetadata{}, cmd.FailErr(err, "get previous image")
	}

	analyzedMD, err := (&lifecycle.Analyzer{
		Buildpacks:            aa.platform06.group.Group,
		Cache:                 aa.platform06.cache,
		Logger:                cmd.DefaultLogger,
		Platform:              aa.platform,
		PreviousImage:         previousImage,
		RunImage:              runImage,
		LayerMetadataRestorer: lifecycle.NewLayerMetadataRestorer(cmd.DefaultLogger, aa.layersDir, aa.platform06.skipLayers),
	}).Analyze()
	if err != nil {
		return platform.AnalyzedMetadata{}, cmd.FailErrCode(err, aa.platform.CodeFor(platform.AnalyzeError), "analyzer")
	}

	return analyzedMD, nil
}

func (aa analyzeArgs) localOrRemote(fromImage string) (imgutil.Image, error) {
	if fromImage == "" {
		return nil, nil
	}

	if aa.useDaemon {
		return local.NewImage(
			fromImage,
			aa.docker,
			local.FromBaseImage(fromImage),
		)
	}

	return remote.NewImage(
		fromImage,
		aa.keychain,
		remote.FromBaseImage(fromImage),
	)
}

func (a *analyzeCmd) platformAPIVersionGreaterThan06() bool {
	return a.platform.API().AtLeast("0.7")
}

func (a *analyzeCmd) restoresLayerMetadata() bool {
	return !a.platformAPIVersionGreaterThan06()
}

func (a *analyzeCmd) supportsRunImage() bool {
	return a.platformAPIVersionGreaterThan06()
}

func (a *analyzeCmd) validateRunImageInput() error {
	if !a.supportsRunImage() && a.runImageRef != "" {
		return errors.New("-run-image is unsupported")
	}
	return nil
}

func (a *analyzeCmd) populateRunImage(stackMD platform.StackMetadata, targetRegistry string) error {
	if !a.supportsRunImage() || a.runImageRef != "" {
		return nil
	}

	var err error
	a.runImageRef, err = stackMD.BestRunImageMirror(targetRegistry)
	if err != nil {
		return errors.New("-run-image is required when there is no stack metadata available")
	}
	return nil
}

func (aa *analyzeArgs) ReadableRegistryImages() []string {
	var readableImages []string
	if !aa.useDaemon {
		readableImages = appendNotEmpty(readableImages, aa.previousImageRef, aa.runImageRef)
	}
	return readableImages
}
func (aa *analyzeArgs) WriteableRegistryImages() []string {
	var writeableImages []string
	writeableImages = appendNotEmpty(writeableImages, aa.cacheImageRef)
	if !aa.useDaemon {
		writeableImages = appendNotEmpty(writeableImages, aa.outputImageRef)
		writeableImages = appendNotEmpty(writeableImages, aa.additionalTags...)
	}
	return writeableImages
}
