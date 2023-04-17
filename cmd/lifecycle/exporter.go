package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layout"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type exportCmd struct {
	*platform.Platform

	docker   client.CommonAPIClient // construct if necessary before dropping privileges
	keychain authn.Keychain         // construct if necessary before dropping privileges

	persistedData exportData
}

type exportData struct {
	analyzedMD platform.AnalyzedMetadata
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (e *exportCmd) DefineFlags() {
	if e.PlatformAPI.AtLeast("0.12") {
		cli.FlagExtendedDir(&e.ExtendedDir)
		cli.FlagLayoutDir(&e.LayoutDir)
		cli.FlagRunPath(&e.RunPath)
		cli.FlagUseLayout(&e.UseLayout)
	} else {
		cli.FlagStackPath(&e.StackPath)
	}
	if e.PlatformAPI.AtLeast("0.11") {
		cli.FlagLauncherSBOMDir(&e.LauncherSBOMDir)
	}
	cli.FlagAnalyzedPath(&e.AnalyzedPath)
	cli.FlagAppDir(&e.AppDir)
	cli.FlagCacheDir(&e.CacheDir)
	cli.FlagCacheImage(&e.CacheImageRef)
	cli.FlagGID(&e.GID)
	cli.FlagGroupPath(&e.GroupPath)
	cli.FlagLaunchCacheDir(&e.LaunchCacheDir)
	cli.FlagLauncherPath(&e.LauncherPath)
	cli.FlagLayersDir(&e.LayersDir)
	cli.FlagProcessType(&e.DefaultProcessType)
	cli.FlagProjectMetadataPath(&e.ProjectMetadataPath)
	cli.FlagReportPath(&e.ReportPath)
	cli.FlagRunImage(&e.RunImageRef) // FIXME: this flag isn't valid on Platform 0.7 and later
	cli.FlagUID(&e.UID)
	cli.FlagUseDaemon(&e.UseDaemon)

	cli.DeprecatedFlagRunImage(&e.DeprecatedRunImageRef) // FIXME: this flag isn't valid on Platform 0.7 and later
}

// Args validates arguments and flags, and fills in default values.
func (e *exportCmd) Args(nargs int, args []string) error {
	if nargs == 0 {
		return cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	e.OutputImageRef = args[0]
	e.AdditionalTags = args[1:]
	if err := platform.ResolveInputs(platform.Export, e.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	// read analyzed metadata for use in later stages
	var err error
	e.persistedData.analyzedMD, err = platform.ReadAnalyzed(e.AnalyzedPath, cmd.DefaultLogger)
	if err != nil {
		return err
	}
	if e.UseLayout {
		if err := platform.GuardExperimental(platform.LayoutFormat, cmd.DefaultLogger); err != nil {
			return err
		}
	}
	return nil
}

func (e *exportCmd) Privileges() error {
	var err error
	e.keychain, err = auth.DefaultKeychain(e.registryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}
	if e.UseDaemon {
		var err error
		e.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err = priv.EnsureOwner(e.UID, e.GID, e.CacheDir, e.LaunchCacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err = priv.RunAs(e.UID, e.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", e.UID, e.GID))
	}
	return nil
}

func (e *exportCmd) Exec() error {
	group, err := lifecycle.ReadGroup(e.GroupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}
	if err = verifyBuildpackApis(group); err != nil {
		return err
	}
	cacheStore, err := initCache(e.CacheImageRef, e.CacheDir, e.keychain)
	if err != nil {
		return err
	}
	if e.hasExtendedLayers() {
		if err := platform.GuardExperimental(platform.FeatureDockerfiles, cmd.DefaultLogger); err != nil {
			return err
		}
	}
	return e.export(group, cacheStore, e.persistedData.analyzedMD)
}

func (e *exportCmd) registryImages() []string {
	registryImages := e.RegistryImages()
	if !e.UseDaemon {
		if e.persistedData.analyzedMD.PreviousImageRef() != "" {
			registryImages = append(registryImages, e.persistedData.analyzedMD.PreviousImageRef())
		}
	}
	return registryImages
}

func (e *exportCmd) export(group buildpack.Group, cacheStore lifecycle.Cache, analyzedMD platform.AnalyzedMetadata) error {
	artifactsDir, err := os.MkdirTemp("", "lifecycle.exporter.layer")

	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	var projectMD platform.ProjectMetadata
	_, err = toml.DecodeFile(e.ProjectMetadataPath, &projectMD)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		cmd.DefaultLogger.Debugf("no project metadata found at path '%s', project metadata will not be exported\n", e.ProjectMetadataPath)
	}

	exporter := &lifecycle.Exporter{
		Buildpacks: group.Group,
		LayerFactory: &layers.Factory{
			ArtifactsDir: artifactsDir,
			UID:          e.UID,
			GID:          e.GID,
			Logger:       cmd.DefaultLogger,
		},
		Logger:      cmd.DefaultLogger,
		PlatformAPI: e.PlatformAPI,
	}

	var (
		appImage   imgutil.Image
		runImageID string
	)
	switch {
	case e.UseLayout:
		appImage, runImageID, err = e.initLayoutAppImage(analyzedMD)
	case e.UseDaemon:
		appImage, runImageID, err = e.initDaemonAppImage(analyzedMD)
	default:
		appImage, runImageID, err = e.initRemoteAppImage(analyzedMD)
	}
	if err != nil {
		return err
	}

	runImageForExport, err := e.getRunImageForExport()
	if err != nil {
		return err
	}

	report, err := exporter.Export(lifecycle.ExportOptions{
		AdditionalNames:    e.AdditionalTags,
		AppDir:             e.AppDir,
		DefaultProcessType: e.DefaultProcessType,
		ExtendedDir:        e.ExtendedDir,
		LauncherConfig:     launcherConfig(e.LauncherPath, e.LauncherSBOMDir),
		LayersDir:          e.LayersDir,
		OrigMetadata:       analyzedMD.Metadata,
		Project:            projectMD,
		RunImageRef:        runImageID,
		RunImageForExport:  runImageForExport,
		Stack:              platform.StackMetadata{RunImage: runImageForExport}, // for backwards compat
		WorkingImage:       appImage,
	})
	if err != nil {
		return cmd.FailErrCode(err, e.CodeFor(platform.ExportError), "export")
	}
	if err = encoding.WriteTOML(e.ReportPath, &report); err != nil {
		return cmd.FailErrCode(err, e.CodeFor(platform.ExportError), "write export report")
	}

	if cacheStore != nil {
		if cacheErr := exporter.Cache(e.LayersDir, cacheStore); cacheErr != nil {
			cmd.DefaultLogger.Warnf("Failed to export cache: %v\n", cacheErr)
		}
	}
	return nil
}

func (e *exportCmd) initDaemonAppImage(analyzedMD platform.AnalyzedMetadata) (imgutil.Image, string, error) {
	if isDigestRef(e.RunImageRef) {
		// If extensions were used to extend the runtime base image, the run image reference will contain a digest.
		// The restorer uses a name reference to pull the image from the registry (because the extender needs a manifest),
		// and writes a digest reference to analyzed.toml.
		// For remote images, this works perfectly well.
		// However for local images, the daemon can't find the image when the reference contains a digest,
		// so we convert the run image reference back into a name reference by removing the digest.
		ref, err := name.ParseReference(e.RunImageRef)
		if err != nil {
			return nil, "", cmd.FailErr(err, "get run image reference")
		}
		e.RunImageRef = ref.Context().RepositoryStr()
	}

	var opts = []local.ImageOption{
		local.FromBaseImage(e.RunImageRef),
	}
	if e.supportsRunImageExtension() {
		extendedConfig, err := e.getExtendedConfig(analyzedMD.RunImage)
		if err != nil {
			return nil, "", cmd.FailErr(err, "get extended image config")
		}
		if extendedConfig != nil {
			opts = append(opts, local.WithConfig(toContainerConfig(extendedConfig)))
		}
	}

	if analyzedMD.PreviousImageRef() != "" {
		cmd.DefaultLogger.Debugf("Reusing layers from image with id '%s'", analyzedMD.PreviousImageRef())
		opts = append(opts, local.WithPreviousImage(analyzedMD.PreviousImageRef()))
	}

	if !e.customSourceDateEpoch().IsZero() {
		opts = append(opts, local.WithCreatedAt(e.customSourceDateEpoch()))
	}

	var appImage imgutil.Image
	appImage, err := local.NewImage(
		e.OutputImageRef,
		e.docker,
		opts...,
	)
	if err != nil {
		return nil, "", cmd.FailErr(err, " image")
	}

	runImageID, err := appImage.Identifier()
	if err != nil {
		return nil, "", cmd.FailErr(err, "get run image ID")
	}

	if e.LaunchCacheDir != "" {
		volumeCache, err := cache.NewVolumeCache(e.LaunchCacheDir)
		if err != nil {
			return nil, "", cmd.FailErr(err, "create launch cache")
		}
		appImage = cache.NewCachingImage(appImage, volumeCache)
	}
	return appImage, runImageID.String(), nil
}

func isDigestRef(ref string) bool {
	digest, err := name.NewDigest(ref)
	if err != nil {
		return false
	}
	return digest.DigestStr() != ""
}

func toContainerConfig(v1C *v1.Config) *container.Config {
	return &container.Config{
		ArgsEscaped:     v1C.ArgsEscaped,
		AttachStderr:    v1C.AttachStderr,
		AttachStdin:     v1C.AttachStdin,
		AttachStdout:    v1C.AttachStdout,
		Cmd:             v1C.Cmd,
		Domainname:      v1C.Domainname,
		Entrypoint:      v1C.Entrypoint,
		Env:             v1C.Env,
		ExposedPorts:    toNATPortSet(v1C.ExposedPorts),
		Healthcheck:     toHealthConfig(v1C.Healthcheck),
		Hostname:        v1C.Hostname,
		Image:           v1C.Image,
		Labels:          v1C.Labels,
		MacAddress:      v1C.MacAddress,
		NetworkDisabled: v1C.NetworkDisabled,
		OnBuild:         v1C.OnBuild,
		OpenStdin:       v1C.OpenStdin,
		Shell:           v1C.Shell,
		StdinOnce:       v1C.StdinOnce,
		StopSignal:      v1C.StopSignal,
		StopTimeout:     nil,
		Tty:             v1C.Tty,
		User:            v1C.User,
		Volumes:         v1C.Volumes,
		WorkingDir:      v1C.WorkingDir,
	}
}

func toHealthConfig(v1H *v1.HealthConfig) *container.HealthConfig {
	if v1H == nil {
		return &container.HealthConfig{}
	}
	return &container.HealthConfig{
		Interval:    v1H.Interval,
		Retries:     v1H.Retries,
		StartPeriod: v1H.StartPeriod,
		Test:        v1H.Test,
		Timeout:     v1H.Timeout,
	}
}

func toNATPortSet(v1Ps map[string]struct{}) nat.PortSet {
	portSet := make(map[nat.Port]struct{})
	for k, v := range v1Ps {
		portSet[nat.Port(k)] = v
	}
	return portSet
}

func (e *exportCmd) initRemoteAppImage(analyzedMD platform.AnalyzedMetadata) (imgutil.Image, string, error) {
	var opts = []remote.ImageOption{
		remote.FromBaseImage(e.RunImageRef),
	}
	if e.supportsRunImageExtension() {
		extendedConfig, err := e.getExtendedConfig(analyzedMD.RunImage)
		if err != nil {
			return nil, "", cmd.FailErr(err, "get extended image config")
		}
		if extendedConfig != nil {
			opts = append(opts, remote.WithConfig(extendedConfig))
		}
	}

	if analyzedMD.PreviousImageRef() != "" {
		cmd.DefaultLogger.Infof("Reusing layers from image '%s'", analyzedMD.PreviousImageRef())
		opts = append(opts, remote.WithPreviousImage(analyzedMD.PreviousImageRef()))
	}

	if !e.customSourceDateEpoch().IsZero() {
		opts = append(opts, remote.WithCreatedAt(e.customSourceDateEpoch()))
	}

	appImage, err := remote.NewImage(
		e.OutputImageRef,
		e.keychain,
		opts...,
	)
	if err != nil {
		return nil, "", cmd.FailErr(err, "create new app image")
	}

	runImage, err := remote.NewImage(e.RunImageRef, e.keychain, remote.FromBaseImage(e.RunImageRef))
	if err != nil {
		return nil, "", cmd.FailErr(err, "access run image")
	}
	runImageID, err := runImage.Identifier()
	if err != nil {
		return nil, "", cmd.FailErr(err, "get run image reference")
	}
	return appImage, runImageID.String(), nil
}

func (e *exportCmd) initLayoutAppImage(analyzedMD platform.AnalyzedMetadata) (imgutil.Image, string, error) {
	runImageIdentifier, err := layout.ParseIdentifier(analyzedMD.RunImage.Reference)
	if err != nil {
		return nil, "", cmd.FailErr(err, "parsing run image reference")
	}

	var opts = []layout.ImageOption{
		layout.FromBaseImagePath(runImageIdentifier.Path),
	}

	if analyzedMD.PreviousImageRef() != "" {
		previousImageReference, err := layout.ParseIdentifier(analyzedMD.PreviousImageRef())
		if err != nil {
			return nil, "", cmd.FailErr(err, "parsing previous image reference")
		}
		cmd.DefaultLogger.Infof("Reusing layers from image '%s'", previousImageReference.Path)
		opts = append(opts, layout.WithPreviousImage(previousImageReference.Path))
	}

	if !e.customSourceDateEpoch().IsZero() {
		opts = append(opts, layout.WithCreatedAt(e.customSourceDateEpoch()))
	}

	outputImageRefPath, err := layout.ParseRefToPath(e.OutputImageRef)
	if err != nil {
		return nil, "", cmd.FailErr(err, "parsing output image reference")
	}
	appPath := filepath.Join(e.LayoutDir, outputImageRefPath)
	cmd.DefaultLogger.Infof("Using app image: %s\n", appPath)

	appImage, err := layout.NewImage(appPath, opts...)
	if err != nil {
		return nil, "", cmd.FailErr(err, "create new app image")
	}

	// set org.opencontainers.image.ref.name
	reference, err := name.ParseReference(e.OutputImageRef, name.WeakValidation)
	if err != nil {
		return nil, "", err
	}
	if err = appImage.AnnotateRefName(reference.Identifier()); err != nil {
		return nil, "", err
	}

	runImage, err := layout.NewImage(runImageIdentifier.Path)
	if err != nil {
		return nil, "", cmd.FailErr(err, "access run image")
	}
	runImageID, err := runImage.Identifier()
	if err != nil {
		return nil, "", cmd.FailErr(err, "get run image reference")
	}
	return appImage, runImageID.String(), nil
}

func launcherConfig(launcherPath, launcherSBOMDir string) lifecycle.LauncherConfig {
	return lifecycle.LauncherConfig{
		Path:    launcherPath,
		SBOMDir: launcherSBOMDir,
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

func (e *exportCmd) customSourceDateEpoch() time.Time {
	if e.PlatformAPI.LessThan("0.9") {
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

func (e *exportCmd) supportsRunImageExtension() bool {
	return e.PlatformAPI.AtLeast("0.12") && !e.UseLayout // FIXME: add layout support as part of https://github.com/buildpacks/lifecycle/issues/1057
}

func (e *exportCmd) getExtendedConfig(runImage *platform.RunImage) (*v1.Config, error) {
	if runImage == nil {
		return nil, errors.New("missing analyzed run image")
	}
	if !runImage.Extend {
		return nil, nil
	}
	extendedImage, _, err := image.FromLayoutPath(filepath.Join(e.ExtendedDir, "run"))
	if err != nil {
		return nil, err
	}
	if extendedImage == nil {
		return nil, errors.New("missing extended run image")
	}
	extendedConfig, err := extendedImage.ConfigFile()
	if err != nil {
		return nil, err
	}
	if extendedConfig == nil {
		return nil, errors.New("missing extended run image config")
	}
	return &extendedConfig.Config, nil
}

func (e *exportCmd) getRunImageForExport() (platform.RunImageForExport, error) {
	if e.PlatformAPI.LessThan("0.12") {
		stackMD, err := platform.ReadStack(e.StackPath, cmd.DefaultLogger)
		if err != nil {
			return platform.RunImageForExport{}, err
		}
		return stackMD.RunImage, nil
	}
	runMD, err := platform.ReadRun(e.RunPath, cmd.DefaultLogger)
	if err != nil {
		return platform.RunImageForExport{}, err
	}
	if len(runMD.Images) == 0 {
		return platform.RunImageForExport{Image: e.RunImageRef}, nil
	}
	runRef, err := name.ParseReference(e.RunImageRef)
	if err != nil {
		return platform.RunImageForExport{}, err
	}
	for _, runImage := range runMD.Images {
		if runImage.Image == runRef.Context().Name() {
			return runImage, nil
		}
	}
	return platform.RunImageForExport{Image: e.RunImageRef}, nil
}

func (e *exportCmd) hasExtendedLayers() bool {
	if e.ExtendedDir == "" {
		return false
	}
	fis, err := os.ReadDir(filepath.Join(e.ExtendedDir, "run"))
	if err != nil {
		return false
	}
	if len(fis) == 0 {
		return false
	}
	return true
}
