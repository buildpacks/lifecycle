package main

import (
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
)

type analyzeCmd struct {
	//flags
	analyzedPath  string
	cacheDir      string
	cacheImageTag string
	groupPath     string
	imageName     string
	layersDir     string
	skipLayers    bool
	useDaemon     bool
	uid           int
	gid           int

	//set if necessary before dropping privileges
	docker client.CommonAPIClient
}

func (a *analyzeCmd) Init() {
	cmd.FlagAnalyzedPath(&a.analyzedPath)
	cmd.FlagCacheDir(&a.cacheDir)
	cmd.FlagCacheImage(&a.cacheImageTag)
	cmd.FlagGroupPath(&a.groupPath)
	cmd.FlagLayersDir(&a.layersDir)
	cmd.FlagSkipLayers(&a.skipLayers)
	cmd.FlagUseDaemon(&a.useDaemon)
	cmd.FlagUID(&a.uid)
	cmd.FlagGID(&a.gid)
}

func (a *analyzeCmd) Args(nargs int, args []string) error {
	if nargs != 1 {
		return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeInvalidArgs, "parse arguments")
	}
	if args[0] == "" {
		return cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}
	a.imageName = args[0]
	return nil
}

func (a *analyzeCmd) DropPrivileges() error {
	if a.useDaemon {
		var err error
		a.docker, err = dockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err := cmd.EnsureOwner(a.uid, a.gid, a.layersDir, a.cacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := cmd.RunAs(a.uid, a.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", a.uid, a.gid))
	}
	return nil
}

func (a *analyzeCmd) Exec() error {
	group, err := lifecycle.ReadGroup(a.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	cacheStore, err := initCache(a.cacheImageTag, a.cacheDir)
	if err != nil {
		return err
	}

	var img imgutil.Image
	if a.useDaemon {
		img, err = local.NewImage(
			a.imageName,
			a.docker,
			local.FromBaseImage(a.imageName),
		)
	} else {
		img, err = remote.NewImage(
			a.imageName,
			auth.EnvKeychain(cmd.EnvRegistryAuth),
			remote.FromBaseImage(a.imageName),
		)
	}
	if err != nil {
		return cmd.FailErr(err, "get previous image")
	}

	analyzedMd, err := (&lifecycle.Analyzer{
		Buildpacks: group.Group,
		LayersDir:  a.layersDir,
		Logger:     cmd.Logger,
		SkipLayers: a.skipLayers,
	}).Analyze(img, cacheStore)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "analyzer")
	}

	if err := lifecycle.WriteTOML(a.analyzedPath, analyzedMd); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}
