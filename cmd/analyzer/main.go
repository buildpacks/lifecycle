package main

import (
	"flag"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/packs"
	"github.com/buildpack/packs/img"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"log"
)

var (
	repoName   string
	useDaemon  bool
	useHelpers bool
	groupPath  string
	launchDir  string
)

func init() {
	packs.InputBPGroupPath(&groupPath)
	packs.InputUseDaemon(&useDaemon)
	packs.InputUseHelpers(&useHelpers)

	flag.StringVar(&launchDir, "launch", "/launch", "launch directory")
}

func main() {
	flag.Parse()
	repoName = flag.Arg(0)
	if flag.NArg() > 1 || repoName == "" || launchDir == "" {
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse arguments"))
	}
	packs.Exit(analyzer())
}

func analyzer() error {
	if useHelpers {
		if err := img.SetupCredHelpers(repoName); err != nil {
			return packs.FailErr(err, "setup credential helpers")
		}
	}

	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return packs.FailErr(err, "read group")
	}

	newRepoStore := img.NewRegistry
	if useDaemon {
		newRepoStore = img.NewDaemon
	}
	repoStore, err := newRepoStore(repoName)
	if err != nil {
		return packs.FailErr(err, "repository configuration", repoName)
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
		return packs.FailErr(err, "access manifest", repoName)
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
		return packs.FailErrCode(err, packs.CodeFailedBuild)
	}

	return nil
}
