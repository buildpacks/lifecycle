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
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	analyzeArgs
	additionalTags  cmd.StringSlice
	analyzedPath    string
	cacheImageRef   string
	legacyCacheDir  string
	legacyGroupPath string
	outputImageRef  string
	stackPath       string
	uid, gid        int
}

// analyzeArgs contains inputs needed when run by creator.
type analyzeArgs struct {
	launchCacheDir   string
	layersDir        string
	previousImageRef string
	runImageRef      string
	skipLayers       bool
	useDaemon        bool

	docker      client.CommonAPIClient // construct if necessary before dropping privileges
	keychain    authn.Keychain         // construct if necessary before dropping privileges
	legacyCache lifecycle.Cache
	legacyGroup buildpack.Group
	platform    Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (a *analyzeCmd) DefineFlags() {
	cmd.FlagAnalyzedPath(&a.analyzedPath)
	cmd.FlagCacheImage(&a.cacheImageRef)
	cmd.FlagGID(&a.gid)
	cmd.FlagLayersDir(&a.layersDir)
	cmd.FlagUID(&a.uid)
	cmd.FlagUseDaemon(&a.useDaemon)
	if a.platform.API().AtLeast("0.9") {
		cmd.FlagLaunchCacheDir(&a.launchCacheDir)
		cmd.FlagSkipLayers(&a.skipLayers)
	}
	if a.platformAPIVersionGreaterThan06() {
		cmd.FlagPreviousImage(&a.previousImageRef)
		cmd.FlagRunImage(&a.runImageRef)
		cmd.FlagStackPath(&a.stackPath)
		cmd.FlagTags(&a.additionalTags)
	} else {
		cmd.FlagCacheDir(&a.legacyCacheDir)
		cmd.FlagGroupPath(&a.legacyGroupPath)
		cmd.FlagSkipLayers(&a.skipLayers)
	}
}

// Args validates arguments and flags, and fills in default values.
func (a *analyzeCmd) Args(nargs int, args []string) error {
	// read args
	if nargs != 1 {
		return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeInvalidArgs, "parse arguments")
	}
	a.outputImageRef = args[0]

	// validate args
	if a.outputImageRef == "" {
		return cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}

	// fill in values
	if a.analyzedPath == cmd.PlaceholderAnalyzedPath {
		a.analyzedPath = cmd.DefaultAnalyzedPath(a.platform.API().String(), a.layersDir)
	}

	if a.legacyGroupPath == cmd.PlaceholderGroupPath {
		a.legacyGroupPath = cmd.DefaultGroupPath(a.platform.API().String(), a.layersDir)
	}

	if a.previousImageRef == "" {
		a.previousImageRef = a.outputImageRef
	}

	// validate flags
	if a.restoresLayerMetadata() {
		if a.cacheImageRef == "" && a.legacyCacheDir == "" {
			cmd.DefaultLogger.Warn("Not restoring cached layer metadata, no cache flag specified.")
		}
	}

	if !a.useDaemon {
		if err := a.ensurePreviousAndTargetHaveSameRegistry(); err != nil {
			return errors.Wrap(err, "ensuring images are on same registry")
		}
	}

	if a.launchCacheDir != "" && !a.useDaemon {
		cmd.DefaultLogger.Warn("Ignoring -launch-cache, only intended for use with -daemon")
		a.launchCacheDir = ""
	}

	if err := image.ValidateDestinationTags(a.useDaemon, append(a.additionalTags, a.outputImageRef)...); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "validate image tag(s)")
	}

	if err := a.populateRunImageIfNeeded(); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "populate run image")
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
	if err := priv.EnsureOwner(a.uid, a.gid, a.layersDir, a.legacyCacheDir, a.launchCacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(a.uid, a.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", a.uid, a.gid))
	}
	return nil
}

func (a *analyzeCmd) registryImages() []string {
	var registryImages []string
	registryImages = append(registryImages, a.ReadableRegistryImages()...)
	return append(registryImages, a.WriteableRegistryImages()...)
}

func (a *analyzeCmd) Exec() error {
	var (
		group      buildpack.Group
		err        error
		cacheStore lifecycle.Cache
	)
	if a.restoresLayerMetadata() {
		group, err = buildpack.ReadGroup(a.legacyGroupPath)
		if err != nil {
			return cmd.FailErr(err, "read buildpack group")
		}
		if err := verifyBuildpackApis(group); err != nil {
			return err
		}
		cacheStore, err = initCache(a.cacheImageRef, a.legacyCacheDir, a.keychain)
		if err != nil {
			return cmd.FailErr(err, "initialize cache")
		}
		a.legacyGroup = group
		a.legacyCache = cacheStore
	}

	analyzedMD, err := a.analyze()
	if err != nil {
		return err
	}

	if err := encoding.WriteTOML(a.analyzedPath, analyzedMD); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}

func (aa analyzeArgs) analyze() (platform.AnalyzedMetadata, error) {
	previousImage, err := aa.localOrRemote(aa.previousImageRef)
	if err != nil {
		return platform.AnalyzedMetadata{}, cmd.FailErr(err, "get previous image")
	}
	if aa.useDaemon && aa.launchCacheDir != "" {
		volumeCache, err := cache.NewVolumeCache(aa.launchCacheDir)
		if err != nil {
			return platform.AnalyzedMetadata{}, cmd.FailErr(err, "create launch cache")
		}
		previousImage = cache.NewCachingImage(previousImage, volumeCache)
	}

	runImage, err := aa.localOrRemote(aa.runImageRef)
	if err != nil {
		return platform.AnalyzedMetadata{}, cmd.FailErr(err, "get previous image")
	}

	analyzedMD, err := (&lifecycle.Analyzer{
		Buildpacks:            aa.legacyGroup.Group,
		Cache:                 aa.legacyCache,
		Logger:                cmd.DefaultLogger,
		Platform:              aa.platform,
		PreviousImage:         previousImage,
		RunImage:              runImage,
		LayerMetadataRestorer: layer.NewMetadataRestorer(cmd.DefaultLogger, aa.layersDir, aa.skipLayers),
		SBOMRestorer: layer.NewSBOMRestorer(layer.SBOMRestorerOpts{
			LayersDir: aa.layersDir,
			Logger:    cmd.DefaultLogger,
			Nop:       aa.skipLayers,
		}, aa.platform.API()),
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

func (a *analyzeCmd) populateRunImageIfNeeded() error {
	if !a.supportsRunImage() || a.runImageRef != "" {
		return nil
	}

	targetRegistry, err := parseRegistry(a.outputImageRef)
	if err != nil {
		return err
	}

	stackMD, err := readStack(a.stackPath)
	if err != nil {
		return err
	}

	a.runImageRef, err = stackMD.BestRunImageMirror(targetRegistry)
	if err != nil {
		return errors.New("-run-image is required when there is no stack metadata available")
	}

	return nil
}

func (a *analyzeCmd) ensurePreviousAndTargetHaveSameRegistry() error {
	if a.previousImageRef == a.outputImageRef {
		return nil
	}
	targetRegistry, err := parseRegistry(a.outputImageRef)
	if err != nil {
		return err
	}
	previousRegistry, err := parseRegistry(a.previousImageRef)
	if err != nil {
		return err
	}
	if previousRegistry != targetRegistry {
		return fmt.Errorf("previous image is on a different registry %s from the exported image %s", previousRegistry, targetRegistry)
	}
	return nil
}

func (a *analyzeCmd) ReadableRegistryImages() []string {
	var readableImages []string
	if !a.useDaemon {
		readableImages = appendNotEmpty(readableImages, a.previousImageRef, a.runImageRef)
	}
	return readableImages
}

func (a *analyzeCmd) WriteableRegistryImages() []string {
	var writeableImages []string
	writeableImages = appendNotEmpty(writeableImages, a.cacheImageRef)
	if !a.useDaemon {
		writeableImages = appendNotEmpty(writeableImages, a.outputImageRef)
		writeableImages = appendNotEmpty(writeableImages, a.additionalTags...)
	}
	return writeableImages
}
