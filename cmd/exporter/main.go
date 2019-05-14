package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/imgutil"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cache"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/docker"
	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/lifecycle/image/auth"
	"github.com/buildpack/lifecycle/metadata"
)

var (
	repoName       string
	runImageRef    string
	layersDir      string
	appDir         string
	groupPath      string
	stackPath      string
	launchCacheDir string
	useDaemon      bool
	useHelpers     bool
	uid            int
	gid            int
)

const launcherPath = "/lifecycle/launcher"

func init() {
	cmd.FlagRunImage(&runImageRef)
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagStackPath(&stackPath)
	cmd.FlagLaunchCacheDir(&launchCacheDir)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()
	if flag.NArg() > 1 {
		cmd.Exit(cmd.FailErrCode(fmt.Errorf("received %d args expected 1", flag.NArg()), cmd.CodeInvalidArgs, "parse arguments"))
	}
	if flag.Arg(0) == "" {
		cmd.Exit(cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments"))
	}

	if launchCacheDir != "" && !useDaemon {
		cmd.Exit(cmd.FailErrCode(errors.New("launch cache can only be used when exporting to a Docker daemon"), cmd.CodeInvalidArgs, "parse arguments"))
	}

	repoName = flag.Arg(0)
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

	outLog := log.New(os.Stdout, "", 0)
	errLog := log.New(os.Stderr, "", 0)
	exporter := &lifecycle.Exporter{
		Buildpacks:   group.Buildpacks,
		Out:          outLog,
		Err:          errLog,
		UID:          uid,
		GID:          gid,
		ArtifactsDir: artifactsDir,
	}

	var stack metadata.StackMetadata
	_, err = toml.DecodeFile(stackPath, &stack)
	if err != nil {
		outLog.Printf("no stack.toml found at path '%s', stack metadata will not be exported\n", stackPath)
	}

	if runImageRef == "" {
		if stack.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-image is required when there is no stack metadata available"), cmd.CodeInvalidArgs, "parse arguments")
		}

		runImageRef, err = runImageFromStackToml(stack)
		if err != nil {
			return err
		}
	}

	if useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), repoName, runImageRef); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	var runImage, origImage imgutil.Image
	if useDaemon {
		dockerClient, err := docker.DefaultClient()
		if err != nil {
			return err
		}

		runImage, err = imgutil.NewLocalImage(runImageRef, dockerClient)
		if err != nil {
			return cmd.FailErr(err, "access run image")
		}

		if launchCacheDir != "" {
			volumeCache, err := cache.NewVolumeCache(launchCacheDir)
			if err != nil {
				return cmd.FailErr(err, "create launch cache")
			}
			runImage = lifecycle.NewCachingImage(runImage, volumeCache)
		}

		origImage, err = imgutil.NewLocalImage(repoName, dockerClient)
		if err != nil {
			return cmd.FailErr(err, "access previous image")
		}
	} else {
		runImage, err = imgutil.NewRemoteImage(runImageRef, auth.DefaultEnvKeychain())
		if err != nil {
			return cmd.FailErr(err, "access run image")
		}
		origImage, err = imgutil.NewRemoteImage(repoName, auth.DefaultEnvKeychain())
		if err != nil {
			return cmd.FailErr(err, "access previous image")
		}
	}

	if err := exporter.Export(layersDir, appDir, runImage, origImage, launcherPath, stack); err != nil {
		return cmd.FailErr(err, "export")
	}

	return nil
}

func runImageFromStackToml(stack metadata.StackMetadata) (string, error) {
	registry, err := image.ParseRegistry(repoName)
	if err != nil {
		return "", cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse image name")
	}

	runImageMirrors := []string{stack.RunImage.Image}
	runImageMirrors = append(runImageMirrors, stack.RunImage.Mirrors...)
	runImageRef, err := image.ByRegistry(registry, runImageMirrors)
	if err != nil {
		return "", cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse mirrors")
	}
	return runImageRef, nil
}
