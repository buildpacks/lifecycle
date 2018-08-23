package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/packs"
	"github.com/buildpack/packs/img"
)

var (
	repoName          string
	runImage          string
	useDaemon         bool
	useHelpers        bool
	groupPath         string
	launchDir         string
	useDaemonRunImage bool
)

func init() {
	packs.InputRunImage(&runImage)
	packs.InputUseDaemon(&useDaemon)
	packs.InputUseHelpers(&useHelpers)
	packs.InputBPGroupPath(&groupPath)

	flag.StringVar(&launchDir, "launch", "/launch", "launch directory")
	flag.BoolVar(&useDaemonRunImage, "daemon-stack", false, "use stack from docker daemon")
}

func main() {
	flag.Parse()
	if flag.NArg() > 1 || flag.Arg(0) == "" || runImage == "" || launchDir == "" {
		args := map[string]interface{}{"narg": flag.NArg, "runImage": runImage, "launchDir": launchDir}
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse arguments", fmt.Sprintf("%+v", args)))
	}
	repoName = flag.Arg(0)
	packs.Exit(export())
}

func export() error {
	if useHelpers {
		if err := img.SetupCredHelpers(repoName, runImage); err != nil {
			return packs.FailErr(err, "setup credential helpers")
		}
	}

	newRepoStore := img.NewRegistry
	if useDaemon {
		newRepoStore = img.NewDaemon
	}
	repoStore, err := newRepoStore(repoName)
	if err != nil {
		return packs.FailErr(err, "access", repoName)
	}

	newRunImageStore := img.NewRegistry
	if useDaemonRunImage {
		newRunImageStore = img.NewDaemon
	}
	stackStore, err := newRunImageStore(runImage)
	if err != nil {
		return packs.FailErr(err, "access", runImage)
	}
	stackImage, err := stackStore.Image()
	if err != nil {
		return packs.FailErr(err, "get image for", runImage)
	}

	origImage, err := repoStore.Image()
	if err != nil {
		origImage = nil
	} else if _, err := origImage.RawManifest(); err != nil {
		// Assume error is due to non-existent image
		// This is necessary for registries
		origImage = nil
	}

	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return packs.FailErr(err, "read group")
	}

	tmpDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return packs.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(tmpDir)

	exporter := &lifecycle.Exporter{
		Buildpacks: group.Buildpacks,
		TmpDir:     tmpDir,
		Out:        os.Stdout,
		Err:        os.Stderr,
	}
	newImage, err := exporter.Export(
		launchDir,
		stackImage,
		origImage,
	)
	if err != nil {
		return packs.FailErrCode(err, packs.CodeFailedBuild)
	}

	if err := repoStore.Write(newImage); err != nil {
		return packs.FailErrCode(err, packs.CodeFailedUpdate, "write")
	}

	return nil
}
