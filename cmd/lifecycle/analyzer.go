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
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	//flags: inputs
	cacheDir      string
	cacheImageTag string
	groupPath     string
	uid, gid      int
	analyzeArgs

	//flags: paths to write data
	analyzedPath string
}

type analyzeArgs struct {
	//inputs needed when run by creator
	imageName  string
	layersDir  string
	skipLayers bool
	useDaemon  bool

	//construct if necessary before dropping privileges
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
	if a.cacheImageTag == "" && a.cacheDir == "" {
		cmd.DefaultLogger.Warn("Not restoring cached layer metadata, no cache flag specified.")
	}
	a.imageName = args[0]
	return nil
}

func (a *analyzeCmd) Privileges() error {
	if a.useDaemon {
		var err error
		a.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err := priv.EnsureOwner(a.uid, a.gid, a.layersDir, a.cacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(a.uid, a.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", a.uid, a.gid))
	}
	return nil
}

func (a *analyzeCmd) Exec() error {
	group, err := lifecycle.ReadGroup(a.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}
	if err := verifyBuildpackApis(group); err != nil {
		return err
	}

	cacheStore, err := initCache(a.cacheImageTag, a.cacheDir)
	if err != nil {
		return cmd.FailErr(err, "initialize cache")
	}

	analyzedMD, err := a.analyze(group, cacheStore)
	if err != nil {
		return err
	}

	if err := lifecycle.WriteTOML(a.analyzedPath, analyzedMD); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}

func (aa analyzeArgs) analyze(group lifecycle.BuildpackGroup, cacheStore lifecycle.Cache) (lifecycle.AnalyzedMetadata, error) {
	var (
		img imgutil.Image
		err error
	)
	if aa.useDaemon {
		img, err = local.NewImage(
			aa.imageName,
			aa.docker,
			local.FromBaseImage(aa.imageName),
		)
	} else {
		img, err = remote.NewImage(
			aa.imageName,
			auth.NewKeychain(cmd.EnvRegistryAuth),
			remote.FromBaseImage(aa.imageName),
		)
	}
	if err != nil {
		return lifecycle.AnalyzedMetadata{}, cmd.FailErr(err, "get previous image")
	}

	analyzedMD, err := (&lifecycle.Analyzer{
		Buildpacks: group.Group,
		LayersDir:  aa.layersDir,
		Logger:     cmd.DefaultLogger,
		SkipLayers: aa.skipLayers,
	}).Analyze(img, cacheStore)
	if err != nil {
		return lifecycle.AnalyzedMetadata{}, cmd.FailErrCode(err, cmd.CodeAnalyzeError, "analyzer")
	}
	return analyzedMD, nil
}
