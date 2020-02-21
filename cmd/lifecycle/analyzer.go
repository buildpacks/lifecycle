package main

import (
	"fmt"

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
	uid           int
	gid           int
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

func (a *analyzeCmd) Exec() error {
	group, err := lifecycle.ReadGroup(a.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	cacheStore, err := initCache(a.cacheImageTag, a.cacheDir)
	if err != nil {
		return err
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
		return cmd.FailErr(err, "get previous image")
	}

	analyzedMd, err := analyzer.Analyze(img, cacheStore)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "analyzer")
	}

	if err := lifecycle.WriteTOML(a.analyzedPath, analyzedMd); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}
