package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/buildpacks/imgutil/layout"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
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
		cli.FlagLayoutDir(&e.LayoutDir)
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
	cli.FlagProcessType(&e.DefaultProcessType)
	cli.FlagProjectMetadataPath(&e.ProjectMetadataPath)
	cli.FlagReportPath(&e.ReportPath)
	cli.FlagRunImage(&e.RunImageRef)
	cli.FlagStackPath(&e.StackPath)
	cli.FlagUID(&e.UID)
	cli.FlagUseDaemon(&e.UseDaemon)

	cli.DeprecatedFlagRunImage(&e.DeprecatedRunImageRef)
}

// Args validates arguments and flags, and fills in default values.
func (e *exportCmd) Args(nargs int, args []string) error {
	if nargs == 0 {
		return cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	e.OutputImageRef = args[0]
	e.AdditionalTags = args[1:]
	if err := platform.ResolveInputs(platform.Export, &e.LifecycleInputs, cmd.DefaultLogger); err != nil {
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
	return e.export(group, cacheStore, e.persistedData.analyzedMD)
}

func (e *exportCmd) registryImages() []string {
	registryImages := e.RegistryImages()
	if !e.UseDaemon {
		if e.persistedData.analyzedMD.PreviousImage != nil {
			registryImages = append(registryImages, e.persistedData.analyzedMD.PreviousImage.Reference)
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

	stackMD, err := platform.ReadStack(e.StackPath, cmd.DefaultLogger)
	if err != nil {
		return err
	}

	report, err := exporter.Export(lifecycle.ExportOptions{
		AdditionalNames:    e.AdditionalTags,
		AppDir:             e.AppDir,
		DefaultProcessType: e.DefaultProcessType,
		LauncherConfig:     launcherConfig(e.LauncherPath, e.LauncherSBOMDir),
		LayersDir:          e.LayersDir,
		OrigMetadata:       analyzedMD.Metadata,
		Project:            projectMD,
		RunImageRef:        runImageID,
		Stack:              stackMD,
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
	var opts = []local.ImageOption{
		local.FromBaseImage(e.RunImageRef),
	}

	if analyzedMD.PreviousImage != nil {
		cmd.DefaultLogger.Debugf("Reusing layers from image with id '%s'", analyzedMD.PreviousImage.Reference)
		opts = append(opts, local.WithPreviousImage(analyzedMD.PreviousImage.Reference))
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

func (e *exportCmd) initRemoteAppImage(analyzedMD platform.AnalyzedMetadata) (imgutil.Image, string, error) {
	var opts = []remote.ImageOption{
		remote.FromBaseImage(e.RunImageRef),
	}

	if analyzedMD.PreviousImage != nil {
		cmd.DefaultLogger.Infof("Reusing layers from image '%s'", analyzedMD.PreviousImage.Reference)
		opts = append(opts, remote.WithPreviousImage(analyzedMD.PreviousImage.Reference))
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
	runImagePath, _ := e.parseLayoutImageReferencce(analyzedMD.RunImage)
	var opts = []layout.ImageOption{
		layout.FromBaseImagePath(runImagePath),
	}

	if analyzedMD.PreviousImage != nil {
		previousImagePath, _ := e.parseLayoutImageReferencce(analyzedMD.PreviousImage)
		cmd.DefaultLogger.Infof("Reusing layers from image '%s'", previousImagePath)
		opts = append(opts, layout.WithPreviousImage(previousImagePath))
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

	runImage, err := layout.NewImage(runImagePath)
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

func (e *exportCmd) parseLayoutImageReferencce(identifier *platform.ImageIdentifier) (string, string) {
	referenceSplit := strings.SplitN(identifier.Reference, "@", 2)
	path := referenceSplit[0]
	digest := referenceSplit[1]
	return path, digest
}
