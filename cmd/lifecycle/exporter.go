package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
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
	"github.com/buildpacks/lifecycle/internal/log"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type exportCmd struct {
	analyzedMD platform.AnalyzedMetadata

	//flags: inputs
	cacheDir              string
	cacheImageTag         string
	groupPath             string
	deprecatedRunImageRef string
	exportArgs

	//flags: paths to write outputs
	analyzedPath string
}

type exportArgs struct {
	// inputs needed when run by creator
	appDir              string
	launchCacheDir      string
	launcherPath        string
	layersDir           string
	processType         string
	projectMetadataPath string
	reportPath          string
	runImageRef         string
	stackPath           string
	targetRegistry      string
	imageNames          []string
	stackMD             platform.StackMetadata

	useDaemon bool
	uid, gid  int

	platform Platform

	// construct if necessary before dropping privileges
	docker   client.CommonAPIClient
	keychain authn.Keychain
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (e *exportCmd) DefineFlags() {
	cmd.FlagAnalyzedPath(&e.analyzedPath)
	cmd.FlagAppDir(&e.appDir)
	cmd.FlagCacheDir(&e.cacheDir)
	cmd.FlagCacheImage(&e.cacheImageTag)
	cmd.FlagGID(&e.gid)
	cmd.FlagGroupPath(&e.groupPath)
	cmd.FlagLaunchCacheDir(&e.launchCacheDir)
	cmd.FlagLauncherPath(&e.launcherPath)
	cmd.FlagLayersDir(&e.layersDir)
	cmd.FlagProcessType(&e.processType)
	cmd.FlagProjectMetadataPath(&e.projectMetadataPath)
	cmd.FlagReportPath(&e.reportPath)
	cmd.FlagRunImage(&e.runImageRef)
	cmd.FlagStackPath(&e.stackPath)
	cmd.FlagUID(&e.uid)
	cmd.FlagUseDaemon(&e.useDaemon)

	cmd.DeprecatedFlagRunImage(&e.deprecatedRunImageRef)
}

// Args validates arguments and flags, and fills in default values.
func (e *exportCmd) Args(nargs int, args []string) error {
	if nargs == 0 {
		return cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}

	e.imageNames = args
	if e.launchCacheDir != "" && !e.useDaemon {
		cmd.DefaultLogger.Warn("Ignoring -launch-cache, only intended for use with -daemon")
		e.launchCacheDir = ""
	}

	if e.cacheImageTag == "" && e.cacheDir == "" {
		cmd.DefaultLogger.Warn("Will not cache data, no cache flag specified.")
	}

	if err := image.ValidateDestinationTags(e.useDaemon, e.imageNames...); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "validate image tag(s)")
	}

	if err := e.validateRunImageInput(); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "validate run image input")
	}

	if e.analyzedPath == cmd.PlaceholderAnalyzedPath {
		e.analyzedPath = cmd.DefaultAnalyzedPath(e.platform.API().String(), e.layersDir)
	}

	if e.groupPath == cmd.PlaceholderGroupPath {
		e.groupPath = cmd.DefaultGroupPath(e.platform.API().String(), e.layersDir)
	}

	if e.projectMetadataPath == cmd.PlaceholderProjectMetadataPath {
		e.projectMetadataPath = cmd.DefaultProjectMetadataPath(e.platform.API().String(), e.layersDir)
	}

	if e.reportPath == cmd.PlaceholderReportPath {
		e.reportPath = cmd.DefaultReportPath(e.platform.API().String(), e.layersDir)
	}

	if e.deprecatedRunImageRef != "" {
		e.runImageRef = e.deprecatedRunImageRef
	}

	var err error
	e.analyzedMD, err = parseAnalyzedMD(cmd.DefaultLogger, e.analyzedPath)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse analyzed metadata")
	}

	e.stackMD, err = readStack(e.stackPath)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse stack metadata")
	}

	e.targetRegistry, err = parseRegistry(e.imageNames[0])
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse target registry")
	}

	if err := e.populateRunImageRefIfNeeded(); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "populate run image")
	}

	return nil
}

func readStack(stackPath string) (platform.StackMetadata, error) {
	var (
		stackMD platform.StackMetadata
	)

	if _, err := toml.DecodeFile(stackPath, &stackMD); err != nil {
		if os.IsNotExist(err) {
			cmd.DefaultLogger.Infof("no stack metadata found at path '%s'\n", stackPath)
		} else {
			return platform.StackMetadata{}, err
		}
	}

	return stackMD, nil
}

func (e *exportCmd) supportsRunImage() bool {
	return e.platform.API().LessThan("0.7")
}

func (e *exportCmd) Privileges() error {
	var err error
	e.keychain, err = auth.DefaultKeychain(e.registryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}

	if e.useDaemon {
		var err error
		e.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err := priv.EnsureOwner(e.uid, e.gid, e.cacheDir, e.launchCacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(e.uid, e.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", e.uid, e.gid))
	}
	return nil
}

func (e *exportCmd) Exec() error {
	group, err := buildpack.ReadGroup(e.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}
	if err := verifyBuildpackApis(group); err != nil {
		return err
	}

	cacheStore, err := initCache(e.cacheImageTag, e.cacheDir, e.keychain)
	if err != nil {
		cmd.DefaultLogger.Infof("no stack metadata found at path '%s', stack metadata will not be exported\n", e.stackPath)
	}

	return e.export(group, cacheStore, e.analyzedMD)
}

func (e *exportCmd) registryImages() []string {
	var registryImages []string
	if e.cacheImageTag != "" {
		registryImages = append(registryImages, e.cacheImageTag)
	}
	if !e.useDaemon {
		registryImages = append(registryImages, e.imageNames...)
		registryImages = append(registryImages, e.runImageRef)
		if e.analyzedMD.PreviousImage != nil {
			registryImages = append(registryImages, e.analyzedMD.PreviousImage.Reference)
		}
	}
	return registryImages
}

func (e *exportCmd) populateRunImageRefIfNeeded() error {
	if !e.supportsRunImage() {
		if e.analyzedMD.RunImage == nil || e.analyzedMD.RunImage.Reference == "" {
			return errors.New("run image not found in analyzed metadata")
		}
		e.runImageRef = e.analyzedMD.RunImage.Reference
	} else if e.runImageRef == "" {
		var err error
		e.runImageRef, err = e.stackMD.BestRunImageMirror(e.targetRegistry)
		if err != nil {
			return errors.New("-run-image is required when there is no stack metadata available")
		}
	}
	return nil
}

func (e *exportCmd) validateRunImageInput() error {
	switch {
	case e.supportsRunImage() && e.deprecatedRunImageRef != "" && e.runImageRef != os.Getenv(cmd.EnvRunImage):
		return errors.New("supply only one of -run-image or (deprecated) -image")
	case !e.supportsRunImage() && e.deprecatedRunImageRef != "":
		return errors.New("-image is unsupported")
	case !e.supportsRunImage() && e.runImageRef != os.Getenv(cmd.EnvRunImage):
		return errors.New("-run-image is unsupported")
	default:
		return nil
	}
}

func (ea exportArgs) export(group buildpack.Group, cacheStore lifecycle.Cache, analyzedMD platform.AnalyzedMetadata) error {
	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	var projectMD platform.ProjectMetadata
	_, err = toml.DecodeFile(ea.projectMetadataPath, &projectMD)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		cmd.DefaultLogger.Debugf("no project metadata found at path '%s', project metadata will not be exported\n", ea.projectMetadataPath)
	}

	exporter := &lifecycle.Exporter{
		Buildpacks: group.Group,
		LayerFactory: &layers.Factory{
			ArtifactsDir: artifactsDir,
			UID:          ea.uid,
			GID:          ea.gid,
			Logger:       cmd.DefaultLogger,
		},
		Logger:      cmd.DefaultLogger,
		PlatformAPI: ea.platform.API(),
	}

	var appImage imgutil.Image
	var runImageID string
	if ea.useDaemon {
		appImage, runImageID, err = ea.initDaemonAppImage(analyzedMD)
	} else {
		appImage, runImageID, err = ea.initRemoteAppImage(analyzedMD)
	}
	if err != nil {
		return err
	}

	report, err := exporter.Export(lifecycle.ExportOptions{
		AdditionalNames:    ea.imageNames[1:],
		AppDir:             ea.appDir,
		DefaultProcessType: ea.processType,
		LauncherConfig:     launcherConfig(ea.launcherPath),
		LayersDir:          ea.layersDir,
		OrigMetadata:       analyzedMD.Metadata,
		Project:            projectMD,
		RunImageRef:        runImageID,
		Stack:              ea.stackMD,
		WorkingImage:       appImage,
	})
	if err != nil {
		return cmd.FailErrCode(err, ea.platform.CodeFor(platform.ExportError), "export")
	}

	if err := encoding.WriteTOML(ea.reportPath, &report); err != nil {
		return cmd.FailErrCode(err, ea.platform.CodeFor(platform.ExportError), "write export report")
	}

	if cacheStore != nil {
		if cacheErr := exporter.Cache(ea.layersDir, cacheStore); cacheErr != nil {
			cmd.DefaultLogger.Warnf("Failed to export cache: %v\n", cacheErr)
		}
	}
	return nil
}

func (ea exportArgs) initDaemonAppImage(analyzedMD platform.AnalyzedMetadata) (imgutil.Image, string, error) {
	var opts = []local.ImageOption{
		local.FromBaseImage(ea.runImageRef),
	}

	if analyzedMD.PreviousImage != nil {
		cmd.DefaultLogger.Debugf("Reusing layers from image with id '%s'", analyzedMD.PreviousImage.Reference)
		opts = append(opts, local.WithPreviousImage(analyzedMD.PreviousImage.Reference))
	}

	if !ea.customSourceDateEpoch().IsZero() {
		opts = append(opts, local.WithCreatedAt(ea.customSourceDateEpoch()))
	}

	var appImage imgutil.Image
	appImage, err := local.NewImage(
		ea.imageNames[0],
		ea.docker,
		opts...,
	)
	if err != nil {
		return nil, "", cmd.FailErr(err, " image")
	}

	runImageID, err := appImage.Identifier()
	if err != nil {
		return nil, "", cmd.FailErr(err, "get run image ID")
	}

	if ea.launchCacheDir != "" {
		volumeCache, err := cache.NewVolumeCache(ea.launchCacheDir)
		if err != nil {
			return nil, "", cmd.FailErr(err, "create launch cache")
		}
		appImage = cache.NewCachingImage(appImage, volumeCache)
	}
	return appImage, runImageID.String(), nil
}

func (ea exportArgs) initRemoteAppImage(analyzedMD platform.AnalyzedMetadata) (imgutil.Image, string, error) {
	var opts = []remote.ImageOption{
		remote.FromBaseImage(ea.runImageRef),
	}

	if analyzedMD.PreviousImage != nil {
		cmd.DefaultLogger.Infof("Reusing layers from image '%s'", analyzedMD.PreviousImage.Reference)
		// ensure previous image is on same registry as output image
		analyzedRegistry, err := parseRegistry(analyzedMD.PreviousImage.Reference)
		if err != nil {
			return nil, "", cmd.FailErr(err, "parse analyzed registry")
		}
		if analyzedRegistry != ea.targetRegistry {
			return nil, "", fmt.Errorf("analyzed image is on a different registry %s from the exported image %s", analyzedRegistry, ea.targetRegistry)
		}
		opts = append(opts, remote.WithPreviousImage(analyzedMD.PreviousImage.Reference))
	}

	if !ea.customSourceDateEpoch().IsZero() {
		opts = append(opts, remote.WithCreatedAt(ea.customSourceDateEpoch()))
	}

	appImage, err := remote.NewImage(
		ea.imageNames[0],
		ea.keychain,
		opts...,
	)
	if err != nil {
		return nil, "", cmd.FailErr(err, "create new app image")
	}

	runImage, err := remote.NewImage(ea.runImageRef, ea.keychain, remote.FromBaseImage(ea.runImageRef))
	if err != nil {
		return nil, "", cmd.FailErr(err, "access run image")
	}
	runImageID, err := runImage.Identifier()
	if err != nil {
		return nil, "", cmd.FailErr(err, "get run image reference")
	}
	return appImage, runImageID.String(), nil
}

func launcherConfig(launcherPath string) lifecycle.LauncherConfig {
	return lifecycle.LauncherConfig{
		Path: launcherPath,
		Metadata: platform.LauncherMetadata{
			Version: cmd.Version,
			Source: platform.SourceMetadata{
				Git: platform.GitMetadata{
					Repository: cmd.SCMRepository,
					Commit:     cmd.SCMCommit,
				},
			},
		},
	}
}

func parseAnalyzedMD(logger log.Logger, path string) (platform.AnalyzedMetadata, error) {
	var analyzedMD platform.AnalyzedMetadata

	_, err := toml.DecodeFile(path, &analyzedMD)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warnf("Warning: analyzed TOML file not found at '%s'", path)
			return platform.AnalyzedMetadata{}, nil
		}

		return platform.AnalyzedMetadata{}, err
	}

	return analyzedMD, nil
}

func parseRegistry(providedRef string) (string, error) {
	ref, err := name.ParseReference(providedRef, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}

func (ea exportArgs) customSourceDateEpoch() time.Time {
	if ea.platform.API().LessThan("0.9") {
		return time.Time{}
	}

	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		seconds, err := strconv.ParseInt(epoch, 10, 64)
		if err != nil {
			cmd.DefaultLogger.Warn("Ignoring invalid SOURCE_DATE_EPOCH")
			return time.Time{}
		}
		return time.Unix(seconds, 0)
	}
	return time.Time{}
}
