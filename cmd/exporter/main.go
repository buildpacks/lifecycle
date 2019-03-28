package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image"
)

var (
	repoName    string
	runImageRef string
	layersDir   string
	appDir      string
	groupPath   string
	stackPath   string
	useDaemon   bool
	useHelpers  bool
	uid         int
	gid         int
)

const launcherPath = "/lifecycle/launcher"

func init() {
	cmd.FlagRunImage(&runImageRef)
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagStackPath(&stackPath)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()
	if flag.NArg() > 1 || flag.Arg(0) == "" || runImageRef == "" {
		args := map[string]interface{}{"narg": flag.NArg(), "runImage": runImageRef, "layersDir": layersDir}
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments", fmt.Sprintf("%+v", args)))
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

	if useHelpers {
		if err := lifecycle.SetupCredHelpers(repoName, runImageRef); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
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

	factory, err := image.NewFactory(image.WithOutWriter(os.Stdout), image.WithEnvKeychain)
	if err != nil {
		return err
	}

	var stack lifecycle.StackMetadata
	_, err = toml.DecodeFile(stackPath, &stack)
	if err != nil {
		outLog.Printf("no stack.toml found at path '%s', stack metadata will not be exported\n", stackPath)
	}

	var runImage, origImage image.Image
	if useDaemon {
		runImage, err = factory.NewLocal(runImageRef)
		if err != nil {
			return err
		}
		origImage, err = factory.NewLocal(repoName)
		if err != nil {
			return err
		}
	} else {
		runImage, err = factory.NewRemote(runImageRef)
		if err != nil {
			return err
		}
		origImage, err = factory.NewRemote(repoName)
		if err != nil {
			return err
		}
	}

	if err := exporter.Export(layersDir, appDir, runImage, origImage, launcherPath, stack); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	return nil
}
