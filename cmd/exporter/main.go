package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/img"
)

var (
	repoName     string
	runImage     string
	launchDir    string
	launchDirSrc string
	dryRun       string
	appDir       string
	appDirSrc    string
	groupPath    string
	useDaemon    bool
	useHelpers   bool
	uid          int
	gid          int
)

func init() {
	cmd.FlagRunImage(&runImage)
	cmd.FlagLaunchDir(&launchDir)
	cmd.FlagLaunchDirSrc(&launchDirSrc)
	cmd.FlagAppDir(&appDir)
	cmd.FlagAppDirSrc(&appDirSrc)
	cmd.FlagDryRunDir(&dryRun)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
}

func main() {
	flag.Parse()
	if flag.NArg() > 1 || flag.Arg(0) == "" || runImage == "" {
		args := map[string]interface{}{"narg": flag.NArg(), "runImage": runImage, "launchDir": launchDir}
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments", fmt.Sprintf("%+v", args)))
	}
	repoName = flag.Arg(0)
	cmd.Exit(export())
}

func export() error {
	var group lifecycle.BuildpackGroup
	var err error
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return cmd.FailErr(err, "read group")
	}

	exporter := &lifecycle.Exporter{
		Buildpacks: group.Buildpacks,
		Out:        os.Stdout,
		Err:        os.Stderr,
		UID:        uid,
		GID:        gid,
	}

	if dryRun != "" {
		exporter.TmpDir = dryRun
		if err := os.MkdirAll(exporter.TmpDir, 0777); err != nil {
			return cmd.FailErr(err, "create temp directory")
		}
	} else {
		exporter.TmpDir, err = ioutil.TempDir("", "lifecycle.exporter.layer")
		if err != nil {
			return cmd.FailErr(err, "create temp directory")
		}
		defer os.RemoveAll(exporter.TmpDir)
	}

	_, err = exporter.PrepareExport(
		launchDirSrc,
		launchDir,
		appDirSrc,
		appDir,
	)
	if err != nil {
		return cmd.FailErr(err, "prepare export")
	}

	if dryRun != "" {
		return nil
	}

	if useHelpers {
		if err := img.SetupCredHelpers(repoName, runImage); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	newRepoStore := img.NewRegistry
	if useDaemon {
		newRepoStore = img.NewDaemon
	}
	repoStore, err := newRepoStore(repoName)
	if err != nil {
		return cmd.FailErr(err, "access", repoName)
	}

	newRunImageStore := img.NewRegistry
	if useDaemon {
		newRunImageStore = img.NewDaemon
	}
	stackStore, err := newRunImageStore(runImage)
	if err != nil {
		return cmd.FailErr(err, "access", runImage)
	}
	stackImage, err := stackStore.Image()
	if err != nil {
		return cmd.FailErr(err, "get image for", runImage)
	}

	origImage, err := repoStore.Image()
	if err != nil {
		origImage = nil
	} else if _, err := origImage.RawManifest(); err != nil {
		// Assume error is due to non-existent image
		// This is necessary for registries
		origImage = nil
	}

	newImage, err := exporter.ExportImage(
		launchDirSrc,
		launchDir,
		appDir,
		stackImage,
		origImage,
	)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	if err := repoStore.Write(newImage); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedUpdate, "write")
	}

	return nil
}
