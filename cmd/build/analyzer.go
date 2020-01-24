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

func analyzer(f analyzeFlags) error {
	if f.useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), f.imageName); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	group, err := lifecycle.ReadGroup(f.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	cacheStore, err := initCache(f.cacheImageTag, f.cacheDir)
	if err != nil {
		return err
	}

	analyzedMd, err := analyze(group, f.imageName, f.layersDir, f.uid, f.gid, cacheStore, f.skipLayers, f.useDaemon)
	if err != nil {
		return err
	}

	if err := lifecycle.WriteTOML(f.analyzedPath, analyzedMd); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}

func analyze(group lifecycle.BuildpackGroup, imageName, layersDir string, uid, gid int, cacheStore lifecycle.Cache, skipLayers, useDaemon bool) (lifecycle.AnalyzedMetadata, error) {
	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Group,
		LayersDir:  layersDir,
		Logger:     cmd.Logger,
		UID:        uid,
		GID:        gid,
		SkipLayers: skipLayers,
	}

	img, err := initImage(imageName, useDaemon)
	if err != nil {
		return lifecycle.AnalyzedMetadata{}, cmd.FailErr(err, "access previous image")
	}

	analyzedMd, err := analyzer.Analyze(img, cacheStore)
	if err != nil {
		return lifecycle.AnalyzedMetadata{}, cmd.FailErrCode(err, cmd.CodeFailed, "analyzer")
	}
	return *analyzedMd, nil
}
