package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
)

type exportCmd struct {
	imageNames          []string
	runImageRef         string
	layersDir           string
	appDir              string
	groupPath           string
	analyzedPath        string
	stackPath           string
	launchCacheDir      string
	launcherPath        string
	useDaemon           bool
	uid                 int
	gid                 int
	cacheImageTag       string
	cacheDir            string
	projectMetadataPath string
	processType         string
}

func (e *exportCmd) Init() {
	cmd.DeprecatedFlagRunImage(&e.runImageRef)
	cmd.FlagLayersDir(&e.layersDir)
	cmd.FlagAppDir(&e.appDir)
	cmd.FlagGroupPath(&e.groupPath)
	cmd.FlagAnalyzedPath(&e.analyzedPath)
	cmd.FlagStackPath(&e.stackPath)
	cmd.FlagLaunchCacheDir(&e.launchCacheDir)
	cmd.FlagUseDaemon(&e.useDaemon)
	cmd.FlagUID(&e.uid)
	cmd.FlagGID(&e.gid)
	cmd.FlagLauncherPath(&e.launcherPath)
	cmd.FlagCacheImage(&e.cacheImageTag)
	cmd.FlagCacheDir(&e.cacheDir)
	cmd.FlagProjectMetadataPath(&e.projectMetadataPath)
	cmd.FlagProcessType(&e.processType)
}

func (e *exportCmd) Args(nargs int, args []string) error {
	if nargs == 0 {
		return cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}

	e.imageNames = args
	if e.launchCacheDir != "" && !e.useDaemon {
		cmd.Logger.Warn("Ignoring -launch-cache, only intended for use with -daemon")
		e.launchCacheDir = ""
	}

	if e.cacheImageTag == "" && e.cacheDir == "" {
		cmd.Logger.Warn("Not restoring cached layer data, no cache flag specified.")
	}

	return nil
}

func (e *exportCmd) Exec() error {
	group, err := lifecycle.ReadGroup(e.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	analyzedMD, err := parseOptionalAnalyzedMD(cmd.Logger, e.analyzedPath)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse analyzed metadata")
	}

	cacheStore, err := initCache(e.cacheImageTag, e.cacheDir)
	if err != nil {
		cmd.Logger.Infof("no stack metadata found at path '%s', stack metadata will not be exported\n", e.stackPath)
	}

	return export(group, e.stackPath, e.imageNames, e.launchCacheDir, e.appDir, e.layersDir, e.launcherPath, e.projectMetadataPath, e.runImageRef, analyzedMD, cacheStore, e.useDaemon, e.uid, e.gid, e.processType)
}

func export(
	group lifecycle.BuildpackGroup,
	stackPath string,
	imageNames []string,
	launchCacheDir string,
	appDir string,
	layersDir string,
	launcherPath string,
	projectMetadataPath string,
	runImageRef string,
	analyzedMD lifecycle.AnalyzedMetadata,
	cacheStore lifecycle.Cache,
	useDaemon bool,
	uid, gid int,
	processType string,
) error {
	registry, err := image.EnsureSingleRegistry(imageNames...)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse arguments")
	}

	stackMD, runImageRef, err := resolveStack(stackPath, runImageRef, registry)
	if err != nil {
		return err
	}

	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	var projectMD lifecycle.ProjectMetadata
	_, err = toml.DecodeFile(projectMetadataPath, &projectMD)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		cmd.Logger.Debugf("no project metadata found at path '%s', project metadata will not be exported\n", projectMetadataPath)
	}

	exporter := &lifecycle.Exporter{
		Buildpacks:   group.Group,
		Logger:       cmd.Logger,
		UID:          uid,
		GID:          gid,
		ArtifactsDir: artifactsDir,
	}

	var appImage imgutil.Image
	var runImageID imgutil.Identifier
	if useDaemon {
		dockerClient, err := cmd.DockerClient()
		if err != nil {
			return err
		}

		var opts = []local.ImageOption{
			local.FromBaseImage(runImageRef),
		}

		if analyzedMD.Image != nil {
			cmd.Logger.Debugf("Reusing layers from image with id '%s'", analyzedMD.Image.Reference)
			opts = append(opts, local.WithPreviousImage(analyzedMD.Image.Reference))
		}

		appImage, err = local.NewImage(
			imageNames[0],
			dockerClient,
			opts...,
		)
		if err != nil {
			return cmd.FailErr(err, " image")
		}

		runImageID, err = appImage.Identifier()
		if err != nil {
			return cmd.FailErr(err, "get run image ID")
		}

		if launchCacheDir != "" {
			volumeCache, err := cache.NewVolumeCache(launchCacheDir)
			if err != nil {
				return cmd.FailErr(err, "create launch cache")
			}
			appImage = cache.NewCachingImage(appImage, volumeCache)
		}
	} else {
		var opts = []remote.ImageOption{
			remote.FromBaseImage(runImageRef),
		}

		if analyzedMD.Image != nil {
			cmd.Logger.Infof("Reusing layers from image '%s'", analyzedMD.Image.Reference)
			ref, err := name.ParseReference(analyzedMD.Image.Reference, name.WeakValidation)
			if err != nil {
				return cmd.FailErr(err, "parse analyzed registry")
			}
			analyzedRegistry := ref.Context().RegistryStr()
			if analyzedRegistry != registry {
				return fmt.Errorf("analyzed image is on a different registry %s from the exported image %s", analyzedRegistry, registry)
			}
			opts = append(opts, remote.WithPreviousImage(analyzedMD.Image.Reference))
		}

		appImage, err = remote.NewImage(
			imageNames[0],
			auth.EnvKeychain(cmd.EnvRegistryAuth),
			opts...,
		)
		if err != nil {
			return cmd.FailErr(err, "new app image")
		}

		runImage, err := remote.NewImage(runImageRef, auth.EnvKeychain(cmd.EnvRegistryAuth), remote.FromBaseImage(runImageRef))
		if err != nil {
			return cmd.FailErr(err, "access run image")
		}
		runImageID, err = runImage.Identifier()
		if err != nil {
			return cmd.FailErr(err, "get run image reference")
		}
	}

	if err := exporter.Export(lifecycle.ExportOptions{
		LayersDir:          layersDir,
		AppDir:             appDir,
		WorkingImage:       appImage,
		RunImageRef:        runImageID.String(),
		OrigMetadata:       analyzedMD.Metadata,
		AdditionalNames:    imageNames[1:],
		LauncherConfig:     launcherConfig(launcherPath),
		Stack:              stackMD,
		Project:            projectMD,
		DefaultProcessType: processType,
	}); err != nil {
		if _, isSaveError := err.(*imgutil.SaveError); isSaveError {
			return cmd.FailErrCode(err, cmd.CodeFailedSave, "export")
		}
		return cmd.FailErr(err, "export")
	}

	if cacheStore != nil {
		if cacheErr := exporter.Cache(layersDir, cacheStore); cacheErr != nil {
			cmd.Logger.Warnf("Failed to export cache: %v\n", cacheErr)
		}
	}
	return nil
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
		cmd.Logger.Infof("no stack metadata found at path '%s', stack metadata will not be exported\n", stackPath)
	}
	if runImageRef == "" {
		if stackMD.RunImage.Image == "" {
			return lifecycle.StackMetadata{}, "", cmd.FailErrCode(errors.New("-image is required when there is no stack metadata available"), cmd.CodeInvalidArgs, "parse arguments")
		}

		runImageRef, err = stackMD.BestRunImageMirror(registry)
		if err != nil {
			return lifecycle.StackMetadata{}, "", err
		}
	}
	return stackMD, runImageRef, nil
}
