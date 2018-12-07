package main

import (
	"flag"
	"github.com/buildpack/lifecycle/image"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
)

var (
	repoName   string
	layersDir  string
	groupPath  string
	useDaemon  bool
	useHelpers bool
)

func init() {
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
}

func main() {
	flag.Parse()
	repoName = flag.Arg(0)
	if flag.NArg() > 1 || repoName == "" {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments"))
	}
	cmd.Exit(analyzer())
}

func analyzer() error {
	if useHelpers {
		if err := lifecycle.SetupCredHelpers(repoName); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return cmd.FailErr(err, "read group")
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Buildpacks,
		Out:        os.Stdout,
		Err:        os.Stderr,
	}

	var err error
	var previousImage image.Image
	factory, err := image.DefaultFactory()
	if err != nil {
		return err
	}

	if useDaemon {
		previousImage, err = factory.NewLocal(repoName, false)
		if err != nil {
			return err
		}
	} else {
		previousImage, err = factory.NewRemote(repoName)
		if err != nil {
			return err
		}
	}
	if err != nil {
		return cmd.FailErr(err, "repository configuration", repoName)
	}

	err = analyzer.Analyze(
		previousImage,
		layersDir,
	)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	return nil
}
