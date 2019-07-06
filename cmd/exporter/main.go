package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/local"
	"github.com/buildpack/imgutil/remote"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cache"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/lifecycle/image/auth"
	"github.com/buildpack/lifecycle/metadata"
)

var (
	imageNames     []string
	runImageRef    string
	layersDir      string
	appDir         string
	groupPath      string
	analyzedPath   string
	stackPath      string
	launchCacheDir string
	useDaemon      bool
	useHelpers     bool
	uid            int
	gid            int
	printVersion   bool
)

const launcherPath = "/lifecycle/launcher"

func init() {
	cmd.FlagRunImage(&runImageRef)
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagAnalyzedPath(&analyzedPath)
	cmd.FlagStackPath(&stackPath)
	cmd.FlagLaunchCacheDir(&launchCacheDir)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
	cmd.FlagVersion(&printVersion)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()

	if printVersion {
		cmd.ExitWithVersion()
	}

	imageNames = flag.Args()

	if len(imageNames) == 0 {
		cmd.Exit(cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments"))
	}

	if launchCacheDir != "" && !useDaemon {
		cmd.Exit(cmd.FailErrCode(errors.New("launch cache can only be used when exporting to a Docker daemon"), cmd.CodeInvalidArgs, "parse arguments"))
	}

	cmd.Exit(export())
}

func export() error {
	var err error

	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return cmd.FailErr(err, "read group")
	}

	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	exporter := &lifecycle.Exporter{
		Buildpacks:   group.Group,
		Out:          log.New(os.Stdout, "", 0),
		Err:          log.New(os.Stderr, "", 0),
		UID:          uid,
		GID:          gid,
		ArtifactsDir: artifactsDir,
	}

	analyzedMD, err := parseOptionalAnalyzedMD(cmd.OutLogger, analyzedPath)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse analyzed metadata")
	}

	var registry string
	if registry, err = image.EnsureSingleRegistry(imageNames...); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse arguments")
	}

	var stackMD metadata.StackMetadata
	_, err = toml.DecodeFile(stackPath, &stackMD)
	if err != nil {
		cmd.OutLogger.Printf("no stack metadata found at path '%s', stack metadata will not be exported\n", stackPath)
	}

	if runImageRef == "" {
		if stackMD.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-image is required when there is no stack metadata available"), cmd.CodeInvalidArgs, "parse arguments")
		}

		runImageRef, err = runImageFromStackToml(stackMD, registry)
		if err != nil {
			return err
		}
	}

	if useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), imageNames[0], runImageRef); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	var appImage imgutil.Image
	if useDaemon {
		dockerClient, err := cmd.DockerClient()
		if err != nil {
			return err
		}

		var opts = []local.ImageOption{
			local.FromBaseImage(runImageRef),
		}

		if analyzedMD.Image != nil {
			cmd.OutLogger.Printf("Reusing layers from image with id '%s'", analyzedMD.Image.Reference)
			opts = append(opts, local.WithPreviousImage(analyzedMD.Image.Reference))
		}

		appImage, err = local.NewImage(
			imageNames[0],
			dockerClient,
			opts...,
		)

		if err != nil {
			return cmd.FailErr(err, "access run image")
		}

		if launchCacheDir != "" {
			volumeCache, err := cache.NewVolumeCache(launchCacheDir)
			if err != nil {
				return cmd.FailErr(err, "create launch cache")
			}
			appImage = lifecycle.NewCachingImage(appImage, volumeCache)
		}
	} else {
		var opts = []remote.ImageOption{
			remote.FromBaseImage(runImageRef),
		}

		if analyzedMD.Image != nil {
			cmd.OutLogger.Printf("Reusing layers from image '%s'", analyzedMD.Image.Reference)
			analyzedRegistry, err := image.ParseRegistry(analyzedMD.Image.Reference)
			if err != nil {
				return cmd.FailErr(err, "parse analyzed registry")
			}
			if analyzedRegistry != registry {
				return fmt.Errorf("analyzed image is on a different registry %s from the exported image %s", analyzedRegistry, registry)
			}
			opts = append(opts, remote.WithPreviousImage(analyzedMD.Image.Reference))
		}

		appImage, err = remote.NewImage(
			imageNames[0],
			auth.DefaultEnvKeychain(),
			opts...,
		)
		if err != nil {
			return cmd.FailErr(err, "access run image")
		}
	}

	launcherConfig := lifecycle.LauncherConfig{
		Path: launcherPath,
		Metadata: metadata.LauncherMetadata{
			Version: cmd.Version,
			Source: metadata.SourceMetadata{
				Git: metadata.GitMetadata{
					Repository: cmd.SCMRepository,
					Commit:     cmd.SCMCommit,
				},
			},
		},
	}

	if err := exporter.Export(layersDir, appDir, appImage, analyzedMD.Metadata, imageNames[1:], launcherConfig, stackMD); err != nil {
		if _, isSaveError := err.(*imgutil.SaveError); isSaveError {
			return cmd.FailErrCode(err, cmd.CodeFailedSave, "export")
		}

		return cmd.FailErr(err, "export")
	}

	return nil
}

func parseOptionalAnalyzedMD(logger *log.Logger, path string) (metadata.AnalyzedMetadata, error) {
	var analyzedMD metadata.AnalyzedMetadata

	_, err := toml.DecodeFile(path, &analyzedMD)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Printf("Warning: analyzed TOML file not found at '%s'", path)
			return metadata.AnalyzedMetadata{}, nil
		}

		return metadata.AnalyzedMetadata{}, err
	}

	return analyzedMD, nil
}

func runImageFromStackToml(stack metadata.StackMetadata, registry string) (string, error) {
	runImageMirrors := []string{stack.RunImage.Image}
	runImageMirrors = append(runImageMirrors, stack.RunImage.Mirrors...)
	runImageRef, err := image.ByRegistry(registry, runImageMirrors)
	if err != nil {
		return "", cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse mirrors")
	}
	return runImageRef, nil
}
