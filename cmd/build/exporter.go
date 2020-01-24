package main

import (
	"flag"
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

type exportFlags struct {
	imageNames     []string
	runImageRef    string
	layersDir      string
	appDir         string
	groupPath      string
	analyzedPath   string
	stackPath      string
	launchCacheDir string
	launcherPath   string
	useDaemon      bool
	useHelpers     bool
	uid            int
	gid            int
	cacheImageTag  string
	cacheDir       string
}

func parseExportFlags() (exportFlags, error) {
	f := exportFlags{}
	cmd.FlagRunImage(&f.runImageRef)
	cmd.FlagLayersDir(&f.layersDir)
	cmd.FlagAppDir(&f.appDir)
	cmd.FlagGroupPath(&f.groupPath)
	cmd.FlagAnalyzedPath(&f.analyzedPath)
	cmd.FlagStackPath(&f.stackPath)
	cmd.FlagLaunchCacheDir(&f.launchCacheDir)
	cmd.FlagUseDaemon(&f.useDaemon)
	cmd.FlagUseCredHelpers(&f.useHelpers)
	cmd.FlagUID(&f.uid)
	cmd.FlagGID(&f.gid)
	cmd.FlagLauncherPath(&f.launcherPath)
	cmd.FlagCacheImage(&f.cacheImageTag)
	cmd.FlagCacheDir(&f.cacheDir)

	flag.Parse()
	commonFlags()
	f.imageNames = flag.Args()

	if len(f.imageNames) == 0 {
		return exportFlags{}, cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}

	if f.launchCacheDir != "" && !f.useDaemon {
		return exportFlags{}, cmd.FailErrCode(errors.New("launch cache can only be used when exporting to a Docker daemon"), cmd.CodeInvalidArgs, "parse arguments")
	}

	if f.cacheImageTag == "" && f.cacheDir == "" {
		cmd.Logger.Warn("Not restoring cached layer data, no cache flag specified.")
	}

	return f, nil
}

func exporter(f exportFlags) error {
	group, err := lifecycle.ReadGroup(f.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	analyzedMD, err := parseOptionalAnalyzedMD(cmd.Logger, f.analyzedPath)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse analyzed metadata")
	}

	var registry string
	if registry, err = image.EnsureSingleRegistry(f.imageNames...); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse arguments")
	}

	cacheStore, err := initCache(f.cacheImageTag, f.cacheDir)
	if err != nil {
		return err
	}

	stackMD, runImageRef, err := resolveStack(f.stackPath, f.runImageRef, registry)
	if err != nil {
		return err
	}

	if f.useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), f.imageNames[0], runImageRef); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	return export(group, stackMD, f.imageNames[0], f.launchCacheDir, f.appDir, f.layersDir, f.launcherPath, runImageRef, registry, analyzedMD, cacheStore, f.useDaemon, f.uid, f.gid)

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

func export(group lifecycle.BuildpackGroup, stackMD lifecycle.StackMetadata, imageName, launchCacheDir, appDir, layersDir, launcherPath, runImageRef, registry string, analyzedMD lifecycle.AnalyzedMetadata, cacheStore lifecycle.Cache, useDaemon bool, uid, gid int) error {
	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

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

		var previousImage string
		if analyzedMD.Image != nil {
			cmd.Logger.Debugf("Reusing layers from image with id '%s'", analyzedMD.Image.Reference)
			previousImage = analyzedMD.Image.Reference
		}

		appImage, err = local.NewImage(
			imageName,
			dockerClient,
			local.FromBaseImage(runImageRef),
			local.WithPreviousImage(previousImage),
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
			imageName,
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

	launcherConfig := lifecycle.LauncherConfig{
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

	//TODO additional names
	if err := exporter.Export(layersDir, appDir, appImage, runImageID.String(), analyzedMD.Metadata, []string{}, launcherConfig, stackMD); err != nil {
		if _, isSaveError := err.(*imgutil.SaveError); isSaveError {
			return cmd.FailErrCode(err, cmd.CodeFailedSave, "export")
		}
		return cmd.FailErr(err, "export")
	}

	if cacheErr := exporter.Cache(layersDir, cacheStore); cacheErr != nil {
		cmd.Logger.Warnf("Failed to export cache: %v\n", cacheErr)
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
