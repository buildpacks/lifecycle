package main

import (
	"fmt"

	"github.com/buildpacks/lifecycle/cmd"
)

type createCmd struct {
	appDir             string
	buildpacksDir      string
	cacheDir           string
	cacheImageTag      string
	imageName          string
	launchCacheDir     string
	launcherPath       string
	layersDir          string
	orderPath          string
	platformDir        string
	previousImage      string
	runImageRef        string
	stackPath          string
	uid, gid           int
	additionalTags     cmd.StringSlice
	skipRestore        bool
	useDaemon          bool
	projectMetdataPath string
	processType        string
}

func (c *createCmd) Init() {
	cmd.FlagAppDir(&c.appDir)
	cmd.FlagBuildpacksDir(&c.buildpacksDir)
	cmd.FlagCacheDir(&c.cacheDir)
	cmd.FlagCacheImage(&c.cacheImageTag)
	cmd.FlagGID(&c.gid)
	cmd.FlagLaunchCacheDir(&c.launchCacheDir)
	cmd.FlagLauncherPath(&c.launcherPath)
	cmd.FlagLayersDir(&c.layersDir)
	cmd.FlagOrderPath(&c.orderPath)
	cmd.FlagPlatformDir(&c.platformDir)
	cmd.FlagPreviousImage(&c.previousImage)
	cmd.FlagRunImage(&c.runImageRef)
	cmd.FlagSkipRestore(&c.skipRestore)
	cmd.FlagStackPath(&c.stackPath)
	cmd.FlagUID(&c.uid)
	cmd.FlagUseDaemon(&c.useDaemon)
	cmd.FlagTags(&c.additionalTags)
	cmd.FlagProjectMetadataPath(&c.projectMetdataPath)
	cmd.FlagProcessType(&c.projectMetdataPath)
}

func (c *createCmd) Args(nargs int, args []string) error {
	if nargs != 1 {
		return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeInvalidArgs, "parse arguments")
	}

	c.imageName = args[0]
	if c.launchCacheDir != "" && !c.useDaemon {
		cmd.Logger.Warn("Ignoring -launch-cache, only intended for use with -daemon")
		c.launchCacheDir = ""
	}

	if c.cacheImageTag == "" && c.cacheDir == "" {
		cmd.Logger.Warn("Not restoring cached layer data, no cache flag specified.")
	}

	if c.previousImage == "" {
		c.previousImage = c.imageName
	}

	return nil
}

func (c *createCmd) Exec() error {
	group, plan, err := detect(c.orderPath, c.platformDir, c.appDir, c.buildpacksDir, c.uid, c.gid)
	if err != nil {
		return err
	}

	cacheStore, err := initCache(c.cacheImageTag, c.cacheDir)
	if err != nil {
		return err
	}

	analyzedMD, err := analyze(group, c.previousImage, c.layersDir, c.uid, c.gid, cacheStore, c.skipRestore, c.useDaemon)
	if err != nil {
		return err
	}

	if !c.skipRestore {
		if err := restore(c.layersDir, c.uid, c.gid, group, cacheStore); err != nil {
			return err
		}
	}

	if err := build(c.appDir, c.layersDir, c.platformDir, c.buildpacksDir, group, plan, c.uid, c.gid); err != nil {
		return err
	}

	return export(
		group,
		c.stackPath,
		append([]string{c.imageName}, c.additionalTags...),
		c.launchCacheDir,
		c.appDir,
		c.layersDir,
		c.launcherPath,
		c.projectMetdataPath,
		c.runImageRef,
		analyzedMD,
		cacheStore,
		c.useDaemon,
		c.uid,
		c.gid,
		c.processType,
	)
}
