package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layout"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/lifecycle/log"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	"github.com/buildpacks/lifecycle/priv"
)

type exportCmd struct {
	*platform.Platform

	docker   client.CommonAPIClient // construct if necessary before dropping privileges
	keychain authn.Keychain         // construct if necessary before dropping privileges

	persistedData exportData
}

type exportData struct {
	analyzedMD files.Analyzed
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (e *exportCmd) DefineFlags() {
	if e.PlatformAPI.AtLeast("0.13") {
		cli.FlagInsecureRegistries(&e.InsecureRegistries)
	}
	if e.PlatformAPI.AtLeast("0.12") {
		cli.FlagExtendedDir(&e.ExtendedDir)
		cli.FlagLayoutDir(&e.LayoutDir)
		cli.FlagRunPath(&e.RunPath)
		cli.FlagUseLayout(&e.UseLayout)
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
	cli.FlagLogLevel(&e.LogLevel)
	cli.FlagNoColor(&e.NoColor)
	cli.FlagParallelExport(&e.ParallelExport)
	cli.FlagProcessType(&e.DefaultProcessType)
	cli.FlagProjectMetadataPath(&e.ProjectMetadataPath)
	cli.FlagReportPath(&e.ReportPath)
	cli.FlagRunImage(&e.RunImageRef) // FIXME: this flag isn't valid on Platform 0.7 and later
	cli.FlagUID(&e.UID)
	cli.FlagUseDaemon(&e.UseDaemon)

	// deprecated
	cli.DeprecatedFlagRunImage(&e.DeprecatedRunImageRef) // FIXME: this flag isn't valid on Platform 0.7 and later
	if e.PlatformAPI.LessThan("0.12") {
		cli.FlagStackPath(&e.StackPath)
	}
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
	e.persistedData.analyzedMD, err = files.Handler.ReadAnalyzed(e.AnalyzedPath, cmd.DefaultLogger)
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
	group, err := files.Handler.ReadGroup(e.GroupPath)
	if err != nil {
		return err
	}
	if err = verifyBuildpackApis(group); err != nil {
		return err
	}
	cacheStore, err := initCache(e.CacheImageRef, e.CacheDir, e.keychain, e.PlatformAPI.LessThan("0.13"))
	if err != nil {
		return err
	}
	if e.hasExtendedLayers() && e.PlatformAPI.LessThan("0.13") {
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

func (e *exportCmd) export(group buildpack.Group, cacheStore phase.Cache, analyzedMD files.Analyzed) error {
	artifactsDir, err := os.MkdirTemp("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	projectMD, err := files.Handler.ReadProjectMetadata(e.ProjectMetadataPath, cmd.DefaultLogger)
	if err != nil {
		return err
	}

	g := new(errgroup.Group)
	var ctx context.Context

	if e.ParallelExport {
		g, ctx = errgroup.WithContext(context.Background())
	}
	exporter := &phase.Exporter{
		Buildpacks: group.Group,
		LayerFactory: &layers.Factory{
			ArtifactsDir: artifactsDir,
			UID:          e.UID,
			GID:          e.GID,
			Logger:       cmd.DefaultLogger,
			Ctx:          ctx,
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
		appImage, runImageID, err = e.initDaemonAppImage(analyzedMD, cmd.DefaultLogger)
	default:
		appImage, runImageID, err = e.initRemoteAppImage(analyzedMD)
	}
	if err != nil {
		return err
	}

	runImageForExport, err := platform.GetRunImageForExport(e.Inputs())
	if err != nil {
		return err
	}

	g.Go(func() error {
		report, err := exporter.Export(phase.ExportOptions{
			AdditionalNames:    e.AdditionalTags,
			AppDir:             e.AppDir,
			DefaultProcessType: e.DefaultProcessType,
			ExtendedDir:        e.ExtendedDir,
			LauncherConfig:     launcherConfig(e.LauncherPath, e.LauncherSBOMDir),
			LayersDir:          e.LayersDir,
			OrigMetadata:       analyzedMD.LayersMetadata,
			Project:            projectMD,
			RunImageRef:        runImageID,
			RunImageForExport:  runImageForExport,
			WorkingImage:       appImage,
		})
		if err != nil {
			return cmd.FailErrCode(err, e.CodeFor(platform.ExportError), "export")
		}
		if err = files.Handler.WriteReport(e.ReportPath, &report); err != nil {
			return cmd.FailErrCode(err, e.CodeFor(platform.ExportError), "write export report")
		}
		return nil
	})

	if !e.ParallelExport {
		if err := g.Wait(); err != nil {
			return err
		}
	}

	g.Go(func() error {
		if cacheStore != nil {
			if cacheErr := exporter.Cache(e.LayersDir, cacheStore); cacheErr != nil {
				cmd.DefaultLogger.Warnf("Failed to export cache: %v\n", cacheErr)
			}
		}
		return nil
	})

	if err = g.Wait(); err != nil {
		return err
	}

	return nil
}

func (e *exportCmd) initDaemonAppImage(analyzedMD files.Analyzed, logger log.Logger) (imgutil.Image, string, error) {
	var opts = []imgutil.ImageOption{
		local.FromBaseImage(e.RunImageRef),
	}
	if e.supportsRunImageExtension() {
		extendedConfig, err := e.getExtendedConfig(analyzedMD.RunImage)
		if err != nil {
			return nil, "", cmd.FailErr(err, "get extended image config")
		}
		if extendedConfig != nil {
			opts = append(opts, local.WithConfig(extendedConfig))
		}
	}

	if e.supportsHistory() {
		opts = append(opts, local.WithHistory())
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
		volumeCache, err := cache.NewVolumeCache(e.LaunchCacheDir, logger)
		if err != nil {
			return nil, "", cmd.FailErr(err, "create launch cache")
		}
		appImage = cache.NewCachingImage(appImage, volumeCache)
	}
	return appImage, runImageID.String(), nil
}

func (e *exportCmd) initRemoteAppImage(analyzedMD files.Analyzed) (imgutil.Image, string, error) {
	var opts = []imgutil.ImageOption{
		remote.FromBaseImage(e.RunImageRef),
	}

	if e.supportsRunImageExtension() {
		extendedConfig, err := e.getExtendedConfig(analyzedMD.RunImage)
		if err != nil {
			return nil, "", cmd.FailErr(err, "get extended image config")
		}
		if extendedConfig != nil {
			cmd.DefaultLogger.Debugf("Using config from extensions...")
			opts = append(opts, remote.WithConfig(extendedConfig))
		}
	}

	if e.supportsHistory() {
		opts = append(opts, remote.WithHistory())
	}

	opts = append(opts, image.GetInsecureOptions(e.InsecureRegistries)...)

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

func (e *exportCmd) initLayoutAppImage(analyzedMD files.Analyzed) (imgutil.Image, string, error) {
	runImageIdentifier, err := layout.ParseIdentifier(analyzedMD.RunImage.Reference)
	if err != nil {
		return nil, "", cmd.FailErr(err, "parsing run image reference")
	}

	var opts = []imgutil.ImageOption{
		layout.FromBaseImagePath(runImageIdentifier.Path),
	}

	if e.supportsHistory() {
		opts = append(opts, layout.WithHistory())
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

func launcherConfig(launcherPath, launcherSBOMDir string) phase.LauncherConfig {
	return phase.LauncherConfig{
		Path:    launcherPath,
		SBOMDir: launcherSBOMDir,
		Metadata: files.LauncherMetadata{
			Version: cmd.Version,
			Source: files.SourceMetadata{
				Git: files.GitMetadata{
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
	return e.PlatformAPI.AtLeast("0.12") && !e.UseLayout // FIXME: add layout support as part of https://github.com/buildpacks/lifecycle/issues/1102
}

func (e *exportCmd) supportsHistory() bool {
	return e.PlatformAPI.AtLeast("0.12")
}

func (e *exportCmd) getExtendedConfig(runImage *files.RunImage) (*v1.Config, error) {
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
