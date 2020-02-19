package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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
	useHelpers          bool
	uid                 int
	gid                 int
	cacheImageTag       string
	cacheDir            string
	projectMetadataPath string
}

func (e *exportCmd) Init() {
	cmd.FlagRunImage(&e.runImageRef)
	cmd.FlagLayersDir(&e.layersDir)
	cmd.FlagAppDir(&e.appDir)
	cmd.FlagGroupPath(&e.groupPath)
	cmd.FlagAnalyzedPath(&e.analyzedPath)
	cmd.FlagStackPath(&e.stackPath)
	cmd.FlagLaunchCacheDir(&e.launchCacheDir)
	cmd.FlagUseDaemon(&e.useDaemon)
	cmd.FlagUseCredHelpers(&e.useHelpers)
	cmd.FlagUID(&e.uid)
	cmd.FlagGID(&e.gid)
	cmd.FlagLauncherPath(&e.launcherPath)
	cmd.FlagCacheImage(&e.cacheImageTag)
	cmd.FlagCacheDir(&e.cacheDir)
	cmd.FlagProjectMetadataPath(&e.projectMetadataPath)
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

	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	exporter := &lifecycle.Exporter{
		Buildpacks:   group.Group,
		Logger:       cmd.Logger,
		UID:          e.uid,
		GID:          e.gid,
		ArtifactsDir: artifactsDir,
	}

	analyzedMD, err := parseOptionalAnalyzedMD(cmd.Logger, e.analyzedPath)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse analyzed metadata")
	}

	var registry string
	if registry, err = image.EnsureSingleRegistry(e.imageNames...); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse arguments")
	}

	var stackMD lifecycle.StackMetadata
	_, err = toml.DecodeFile(e.stackPath, &stackMD)
	if err != nil {
		cmd.Logger.Infof("no stack metadata found at path '%s', stack metadata will not be exported\n", e.stackPath)
	}

	var projectMD lifecycle.ProjectMetadata
	_, err = toml.DecodeFile(e.projectMetadataPath, &projectMD)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		cmd.Logger.Infof("no project metadata found at path '%s', project metadata will not be exported\n", e.projectMetadataPath)
	}

	if e.runImageRef == "" {
		if stackMD.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-image is required when there is no stack metadata available"), cmd.CodeInvalidArgs, "parse arguments")
		}

		e.runImageRef, err = stackMD.BestRunImageMirror(registry)
		if err != nil {
			return err
		}
	}

	if e.useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), e.imageNames[0], e.runImageRef); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	var appImage imgutil.Image
	var runImageID imgutil.Identifier

	if e.useDaemon {
		dockerClient, err := cmd.DockerClient()
		if err != nil {
			return err
		}

		var opts = []local.ImageOption{
			local.FromBaseImage(e.runImageRef),
		}

		if analyzedMD.Image != nil {
			cmd.Logger.Infof("Reusing layers from image '%s'", analyzedMD.Image.Reference)
			opts = append(opts, local.WithPreviousImage(analyzedMD.Image.Reference))
		}

		appImage, err = local.NewImage(
			e.imageNames[0],
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

		if e.launchCacheDir != "" {
			volumeCache, err := cache.NewVolumeCache(e.launchCacheDir)
			if err != nil {
				return cmd.FailErr(err, "create launch cache")
			}
			appImage = cache.NewCachingImage(appImage, volumeCache)
		}
	} else {
		var opts = []remote.ImageOption{
			remote.FromBaseImage(e.runImageRef),
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
			e.imageNames[0],
			auth.EnvKeychain(cmd.EnvRegistryAuth),
			opts...,
		)
		if err != nil {
			return cmd.FailErr(err, "new app image")
		}

		runImage, err := remote.NewImage(e.runImageRef, auth.EnvKeychain(cmd.EnvRegistryAuth), remote.FromBaseImage(e.runImageRef))
		if err != nil {
			return cmd.FailErr(err, "access run image")
		}
		runImageID, err = runImage.Identifier()
		if err != nil {
			return cmd.FailErr(err, "get run image reference")
		}
	}

	launcherConfig := lifecycle.LauncherConfig{
		Path: e.launcherPath,
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

	if err := exporter.Export(lifecycle.ExportOptions{
		LayersDir:       e.layersDir,
		AppDir:          e.appDir,
		WorkingImage:    appImage,
		RunImageRef:     runImageID.String(),
		OrigMetadata:    analyzedMD.Metadata,
		AdditionalNames: e.imageNames,
		LauncherConfig:  launcherConfig,
		Stack:           stackMD,
		Project:         projectMD,
	}); err != nil {
		if _, isSaveError := err.(*imgutil.SaveError); isSaveError {
			return cmd.FailErrCode(err, cmd.CodeFailedSave, "export")
		}
		return cmd.FailErr(err, "export")
	}

	cacheStore, err := initCache(e.cacheImageTag, e.cacheDir)
	if err != nil {
		return err
	}
	// Failing to export cache should not be an error if the app image export was successful.
	if cacheStore != nil {
		if cacheErr := exporter.Cache(e.layersDir, cacheStore); cacheErr != nil {
			cmd.Logger.Warnf("Failed to export cache: %v\n", cacheErr)
		}
	}
	return nil
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
