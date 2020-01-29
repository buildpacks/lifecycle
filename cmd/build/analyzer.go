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

type analyzeFlags struct {
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

func parseAnalyzeFlags() (analyzeFlags, error) {
	f := analyzeFlags{}
	cmd.FlagAnalyzedPath(&f.analyzedPath)
	cmd.FlagCacheDir(&f.cacheDir)
	cmd.FlagCacheImage(&f.cacheImageTag)
	cmd.FlagGroupPath(&f.groupPath)
	cmd.FlagLayersDir(&f.layersDir)
	cmd.FlagSkipLayers(&f.skipLayers)
	cmd.FlagUseCredHelpers(&f.useHelpers)
	cmd.FlagUseDaemon(&f.useDaemon)
	cmd.FlagUID(&f.uid)
	cmd.FlagGID(&f.gid)
	flag.Parse()
	commonFlags()

	if flag.NArg() > 1 {
		return analyzeFlags{}, cmd.FailErrCode(fmt.Errorf("received %d args expected 1", flag.NArg()), cmd.CodeInvalidArgs, "parse arguments")
	}
	if flag.Arg(0) == "" {
		return analyzeFlags{}, cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}
	f.imageName = flag.Arg(0)

	return f, nil
}

func analyze(f analyzeFlags) error {
	if f.useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), f.imageName); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	group, err := lifecycle.ReadGroup(f.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Group,
		LayersDir:  f.layersDir,
		Logger:     cmd.Logger,
		UID:        f.uid,
		GID:        f.gid,
		SkipLayers: f.skipLayers,
	}

	img, err := initImage(f.imageName, f.useDaemon)
	if err != nil {
		return cmd.FailErr(err, "access previous image")
	}

	cacheStore, err := initCache(f.cacheImageTag, f.cacheDir)
	if err != nil {
		return err
	}

	md, err := analyzer.Analyze(img, cacheStore)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "analyze")
	}

	if err := lifecycle.WriteTOML(f.analyzedPath, md); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}
