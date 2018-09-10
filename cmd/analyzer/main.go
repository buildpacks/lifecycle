package main

import (
	"flag"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/img"
)

var (
	repoName   string
	launchDir  string
	groupPath  string
	useDaemon  bool
	useHelpers bool
)

func init() {
	cmd.FlagLaunchDir(&launchDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseHelpers(&useHelpers)
}

func main() {
	flag.Parse()
	repoName = flag.Arg(0)
	if flag.NArg() > 1 || repoName == "" || launchDir == "" {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments"))
	}
	cmd.Exit(analyzer())
}

func analyzer() error {
	if useHelpers {
		if err := img.SetupCredHelpers(repoName); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return cmd.FailErr(err, "read group")
	}

	newRepoStore := img.NewRegistry
	if useDaemon {
		newRepoStore = img.NewDaemon
	}
	repoStore, err := newRepoStore(repoName)
	if err != nil {
		return cmd.FailErr(err, "repository configuration", repoName)
	}

	origImage, err := repoStore.Image()
	if err != nil {
		log.Printf("WARNING: skipping analyze, authenticating to registry failed: %s", err.Error())
		return nil

	}
	if _, err := origImage.RawManifest(); err != nil {
		if remoteErr, ok := err.(*remote.Error); ok && len(remoteErr.Errors) > 0 {
			switch remoteErr.Errors[0].Code {
			case remote.UnauthorizedErrorCode, remote.ManifestUnknownErrorCode:
				log.Printf("WARNING: skipping analyze, image not found or requires authentication to access: %s", remoteErr.Error())
				return nil
			}
		}
		return cmd.FailErr(err, "access manifest", repoName)
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Buildpacks,
		Out:        os.Stdout,
		Err:        os.Stderr,
	}
	err = analyzer.Analyze(
		launchDir,
		origImage,
	)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	return nil
}
