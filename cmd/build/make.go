package main

import (
	"errors"
	"flag"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/cmd"
)

type makeFlags struct {
	appDir         string
	buildpacksDir  string
	cacheDir       string
	cacheImageTag  string
	gid            int
	imageName      string
	launchCacheDir string
	launcherPath   string
	layersDir      string
	orderPath      string
	platformDir    string
	previousImage  string
	runImageRef    string
	skipRestore    bool
	stackPath      string
	tags           []string
	uid            int
	useDaemon      bool
}

func parseMakeFlags() (makeFlags, error) {
	f := makeFlags{}
	var tagSlice cmd.StringSlice
	cmd.FlagAppDir(&f.appDir)
	cmd.FlagBuildpacksDir(&f.buildpacksDir)
	cmd.FlagCacheDir(&f.cacheDir)
	cmd.FlagCacheImage(&f.cacheImageTag)
	cmd.FlagGID(&f.gid)
	cmd.FlagLaunchCacheDir(&f.launchCacheDir)
	cmd.FlagLauncherPath(&f.launcherPath)
	cmd.FlagLayersDir(&f.layersDir)
	cmd.FlagOrderPath(&f.orderPath)
	cmd.FlagPlatformDir(&f.platformDir)
	cmd.FlagPreviousImage(&f.previousImage)
	cmd.FlagRunImage(&f.runImageRef)
	cmd.FlagSkipRestore(&f.skipRestore)
	cmd.FlagStackPath(&f.stackPath)
	cmd.FlagTags(&tagSlice)
	cmd.FlagUID(&f.uid)
	cmd.FlagUseDaemon(&f.useDaemon)
	flag.Parse()
	f.tags = tagSlice

	commonFlags()
	f.imageName = flag.Arg(0)

	if f.imageName == "" {
		return makeFlags{}, cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}

	if f.previousImage == "" {
		f.previousImage = f.imageName
	}

	if f.launchCacheDir != "" && !f.useDaemon {
		return makeFlags{}, cmd.FailErrCode(errors.New("launch cache can only be used when exporting to a Docker daemon"), cmd.CodeInvalidArgs, "parse arguments")
	}

	if f.cacheImageTag == "" && f.cacheDir == "" {
		cmd.Logger.Warn("Not restoring cached layer data, no cache flag specified.")
	}

	//TODO deal with tags

	return f, nil
}

func doMake(f makeFlags) error {
	group, plan, err := detect(f.orderPath, f.platformDir, f.appDir, f.buildpacksDir)
	if err != nil {
		return err
	}

	cacheStore, err := initCache(f.cacheImageTag, f.cacheDir)
	if err != nil {
		return err
	}

	analyzedMD, err := analyze(group, f.previousImage, f.layersDir, f.uid, f.gid, cacheStore, f.skipRestore, f.useDaemon)
	if err != nil {
		return err
	}

	if !f.skipRestore {
		if err := restore(f.layersDir, f.uid, f.gid, group, cacheStore); err != nil {
			return err
		}
	}

	if err := build(f.appDir, f.layersDir, f.platformDir, f.buildpacksDir, group, plan); err != nil {
		return err
	}

	ref, err := name.ParseReference(f.imageName, name.WeakValidation)
	if err != nil {
		return err
	}
	registry := ref.Context().RegistryStr()

	stackMD, runImageRef, err := resolveStack(f.stackPath, f.runImageRef, registry)
	if err != nil {
		return err
	}

	return export(group, stackMD, f.imageName, f.launchCacheDir, f.appDir, f.layersDir, f.launcherPath, runImageRef, registry, analyzedMD, cacheStore, f.useDaemon, f.uid, f.gid)
}
