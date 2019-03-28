package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image"
)

var (
	repoName   string
	layersDir  string
	appDir     string
	groupPath  string
	useDaemon  bool
	useHelpers bool
	uid        int
	gid        int
)

func init() {
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

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
		AppDir:     appDir,
		LayersDir:  layersDir,
		Out:        log.New(os.Stdout, "", 0),
		Err:        log.New(os.Stderr, "", 0),
		UID:        uid,
		GID:        gid,
	}

	var err error
	var previousImage image.Image
	factory, err := image.NewFactory(image.WithOutWriter(os.Stdout), image.WithEnvKeychain)
	if err != nil {
		return err
	}

	if useDaemon {
		previousImage, err = factory.NewLocal(repoName)
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
	)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	return nil
}
