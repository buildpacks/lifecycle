package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/priv"
)

type exportCmd struct {
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
	imageNames          []string
	launchCacheDir      string
	launcherPath        string
	layersDir           string
	platformAPI         string
	processType         string
	projectMetadataPath string
	reportPath          string
	runImageRef         string
	stackPath           string
	useDaemon           bool
	uid, gid            int

	//construct if necessary before dropping privileges
	docker client.CommonAPIClient
}

func (e *exportCmd) Init() {
	cmd.FlagLayersDir(&e.layersDir)
	cmd.FlagAnalyzedPath(e.platformAPI, e.layersDir, &e.analyzedPath)
	cmd.FlagAppDir(&e.appDir)
	cmd.FlagCacheDir(&e.cacheDir)
	cmd.FlagCacheImage(&e.cacheImageTag)
	cmd.FlagGID(&e.gid)
	cmd.FlagGroupPath(e.platformAPI, e.layersDir, &e.groupPath)
	cmd.FlagLaunchCacheDir(&e.launchCacheDir)
	cmd.FlagLauncherPath(&e.launcherPath)
	cmd.FlagProcessType(&e.processType)
	cmd.FlagProjectMetadataPath(e.platformAPI, e.layersDir, &e.projectMetadataPath)
	cmd.FlagReportPath(e.platformAPI, e.layersDir, &e.reportPath)
	cmd.FlagRunImage(&e.runImageRef)
	cmd.FlagStackPath(&e.stackPath)
	cmd.FlagUID(&e.uid)
	cmd.FlagUseDaemon(&e.useDaemon)

	cmd.DeprecatedFlagRunImage(&e.deprecatedRunImageRef)
}

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

	if e.deprecatedRunImageRef != "" && e.runImageRef != os.Getenv(cmd.EnvRunImage) {
		return cmd.FailErrCode(errors.New("supply only one of -run-image or (deprecated) -image"), cmd.CodeInvalidArgs, "parse arguments")
	}

	if e.deprecatedRunImageRef != "" {
		e.runImageRef = e.deprecatedRunImageRef
	}

	return nil
}

func (e *exportCmd) Privileges() error {
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
	group, err := lifecycle.ReadGroup(e.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}
	if err := verifyBuildpackApis(group); err != nil {
		return err
	}

	analyzedMD, err := parseOptionalAnalyzedMD(cmd.DefaultLogger, e.analyzedPath)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse analyzed metadata")
	}

	cacheStore, err := initCache(e.cacheImageTag, e.cacheDir)
	if err != nil {
		cmd.DefaultLogger.Infof("no stack metadata found at path '%s', stack metadata will not be exported\n", e.stackPath)
	}

	return e.export(group, cacheStore, analyzedMD)
}

func (ea exportArgs) export(group lifecycle.BuildpackGroup, cacheStore lifecycle.Cache, analyzedMD lifecycle.AnalyzedMetadata) error {
	ref, err := name.ParseReference(ea.imageNames[0], name.WeakValidation)
	if err != nil {
		return cmd.FailErr(err, "failed to parse registry")
	}
	registry := ref.Context().RegistryStr()

	stackMD, runImageRef, err := resolveStack(ea.stackPath, ea.runImageRef, registry)
	if err != nil {
		return err
	}

	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	var projectMD lifecycle.ProjectMetadata
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
		PlatformAPI: api.MustParse(ea.platformAPI),
	}

	var appImage imgutil.Image
	var runImageID string
	if ea.useDaemon {
		appImage, runImageID, err = initDaemonImage(
			ea.imageNames[0],
			runImageRef,
			analyzedMD,
			ea.launchCacheDir,
			ea.docker,
		)
	} else {
		appImage, runImageID, err = initRemoteImage(
			ea.imageNames[0],
			runImageRef,
			analyzedMD,
			registry,
		)
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
		Stack:              stackMD,
		WorkingImage:       appImage,
	})
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeExportError, "export")
	}
	if err := lifecycle.WriteTOML(ea.reportPath, &report); err != nil {
		return cmd.FailErrCode(err, cmd.CodeExportError, "write export report")
	}

	if cacheStore != nil {
		if cacheErr := exporter.Cache(ea.layersDir, cacheStore); cacheErr != nil {
			cmd.DefaultLogger.Warnf("Failed to export cache: %v\n", cacheErr)
		}
	}
	return nil
}

func initDaemonImage(imagName string, runImageRef string, analyzedMD lifecycle.AnalyzedMetadata, launchCacheDir string, docker client.CommonAPIClient) (imgutil.Image, string, error) {
	var opts = []local.ImageOption{
		local.FromBaseImage(runImageRef),
	}

	if analyzedMD.Image != nil {
		cmd.DefaultLogger.Debugf("Reusing layers from image with id '%s'", analyzedMD.Image.Reference)
		opts = append(opts, local.WithPreviousImage(analyzedMD.Image.Reference))
	}

	appImage, err := local.NewImage(
		imagName,
		docker,
		opts...,
	)
	if err != nil {
		return nil, "", cmd.FailErr(err, " image")
	}

	runImageID, err := appImage.Identifier()
	if err != nil {
		return nil, "", cmd.FailErr(err, "get run image ID")
	}

	if launchCacheDir != "" {
		volumeCache, err := cache.NewVolumeCache(launchCacheDir)
		if err != nil {
			return nil, "", cmd.FailErr(err, "create launch cache")
		}
		appImage = cache.NewCachingImage(appImage, volumeCache)
	}
	return appImage, runImageID.String(), nil
}

func initRemoteImage(imageName string, runImageRef string, analyzedMD lifecycle.AnalyzedMetadata, registry string) (imgutil.Image, string, error) {
	var opts = []remote.ImageOption{
		remote.FromBaseImage(runImageRef),
	}

	if analyzedMD.Image != nil {
		cmd.DefaultLogger.Infof("Reusing layers from image '%s'", analyzedMD.Image.Reference)
		ref, err := name.ParseReference(analyzedMD.Image.Reference, name.WeakValidation)
		if err != nil {
			return nil, "", cmd.FailErr(err, "parse analyzed registry")
		}
		analyzedRegistry := ref.Context().RegistryStr()
		if analyzedRegistry != registry {
			return nil, "", fmt.Errorf("analyzed image is on a different registry %s from the exported image %s", analyzedRegistry, registry)
		}
		opts = append(opts, remote.WithPreviousImage(analyzedMD.Image.Reference))
	}

	appImage, err := remote.NewImage(
		imageName,
		auth.NewKeychain(cmd.EnvRegistryAuth),
		opts...,
	)
	if err != nil {
		return nil, "", cmd.FailErr(err, "new app image")
	}

	runImage, err := remote.NewImage(runImageRef, auth.NewKeychain(cmd.EnvRegistryAuth), remote.FromBaseImage(runImageRef))
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
		Metadata: lifecycle.LauncherMetadata{
			Version: cmd.Version,
			Source: lifecycle.SourceMetadata{
				Git: lifecycle.GitMetadata{
					Repository: cmd.SCMRepository,
					Commit:     cmd.SCMCommit,
				},
			},
		},
	}
}

func parseOptionalAnalyzedMD(logger lifecycle.Logger, path string) (lifecycle.AnalyzedMetadata, error) {
	var analyzedMD lifecycle.AnalyzedMetadata

	_, err := toml.DecodeFile(path, &analyzedMD)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warnf("Warning: analyzed TOML file not found at '%s'", path)
			return lifecycle.AnalyzedMetadata{}, nil
		}

		return lifecycle.AnalyzedMetadata{}, err
	}

	return analyzedMD, nil
}

func resolveStack(stackPath, runImageRef, registry string) (lifecycle.StackMetadata, string, error) {
	var stackMD lifecycle.StackMetadata
	_, err := toml.DecodeFile(stackPath, &stackMD)
	if err != nil {
		cmd.DefaultLogger.Infof("no stack metadata found at path '%s', stack metadata will not be exported\n", stackPath)
	}
	if runImageRef == "" {
		if stackMD.RunImage.Image == "" {
			return lifecycle.StackMetadata{}, "", cmd.FailErrCode(errors.New("-run-image is required when there is no stack metadata available"), cmd.CodeInvalidArgs, "parse arguments")
		}
		runImageRef, err = stackMD.BestRunImageMirror(registry)
		if err != nil {
			return lifecycle.StackMetadata{}, "", err
		}
	}
	return stackMD, runImageRef, nil
}
