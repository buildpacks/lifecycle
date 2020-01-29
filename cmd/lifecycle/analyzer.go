package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

type analyzeCmd struct {
	analyzedPath  string
	cacheDir      string
	cacheImageTag string
	groupPath     string
	imageName     string
	layersDir     string
	skipLayers    bool
	useDaemon     bool
	useHelpers    bool
	uid           int
	gid           int
}

func (a *analyzeCmd) Flags() {
	cmd.FlagAnalyzedPath(&a.analyzedPath)
	cmd.FlagCacheDir(&a.cacheDir)
	cmd.FlagCacheImage(&a.cacheImageTag)
	cmd.FlagGroupPath(&a.groupPath)
	cmd.FlagLayersDir(&a.layersDir)
	cmd.FlagSkipLayers(&a.skipLayers)
	cmd.FlagUseCredHelpers(&a.useHelpers)
	cmd.FlagUseDaemon(&a.useDaemon)
	cmd.FlagUID(&a.uid)
	cmd.FlagGID(&a.gid)
	flag.Parse()
}

func (a *analyzeCmd) Args() error {
	if flag.NArg() > 1 {
		return cmd.FailErrCode(fmt.Errorf("received %d Args expected 1", flag.NArg()), cmd.CodeInvalidArgs, "parse arguments")
	}
	if flag.Arg(0) == "" {
		return cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}
	a.imageName = flag.Arg(0)

	return nil
}

func (a *analyzeCmd) Exec() error {
	if a.useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), a.imageName); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	group, err := lifecycle.ReadGroup(a.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Group,
		LayersDir:  a.layersDir,
		Logger:     cmd.Logger,
		UID:        a.uid,
		GID:        a.gid,
		SkipLayers: a.skipLayers,
	}

	img, err := initImage(a.imageName, a.useDaemon)
	if err != nil {
		return cmd.FailErr(err, "access previous image")
	}

	cacheStore, err := initCache(a.cacheImageTag, a.cacheDir)
	if err != nil {
		return err
	}

	md, err := analyzer.Analyze(img, cacheStore)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "analyze")
	}

	if err := lifecycle.WriteTOML(a.analyzedPath, md); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}
