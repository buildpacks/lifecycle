package main

import (
	"fmt"
	"log"
	"os"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/env"
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
	fmt.Println("---> DETECTING")
	group, plan, err := detect(c.orderPath, c.platformDir, c.appDir, c.buildpacksDir, c.uid, c.gid)
	if err != nil {
		return err
	}

	cacheStore, err := initCache(c.cacheImageTag, c.cacheDir)
	if err != nil {
		return err
	}

	previousImage, err := initImage(c.imageName, c.useDaemon)
	if err != nil {
		return err
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Group,
		LayersDir:  c.layersDir,
		Logger:     cmd.Logger,
		UID:        c.uid,
		GID:        c.gid,
		SkipLayers: c.skipRestore,
	}

	fmt.Println("---> ANALYZING")
	analyzedMD, err := analyzer.Analyze(previousImage, cacheStore)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "analyzer")
	}

	if !c.skipRestore {
		fmt.Println("---> RESTORING")
		if err := restore(c.layersDir, c.uid, c.gid, group, cacheStore); err != nil {
			return err
		}
	}

	fmt.Println("---> BUILDING")
	builder := &lifecycle.Builder{
		AppDir:        c.appDir,
		LayersDir:     c.layersDir,
		PlatformDir:   c.platformDir,
		BuildpacksDir: c.buildpacksDir,
		Env:           env.NewBuildEnv(os.Environ()),
		Group:         group,
		Plan:          plan,
		Out:           log.New(os.Stdout, "", 0),
		Err:           log.New(os.Stderr, "", 0),
		UID:           c.uid,
		GID:           c.gid,
	}

	md, err := builder.Build()
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild, "build")
	}

	if err := lifecycle.WriteTOML(lifecycle.MetadataFilePath(c.layersDir), md); err != nil {
		return cmd.FailErr(err, "write metadata")
	}

	return export(exportArgs{
		group:               group,
		stackPath:           c.stackPath,
		imageNames:          append([]string{c.imageName}, c.additionalTags...),
		launchCacheDir:      c.launchCacheDir,
		appDir:              c.appDir,
		layersDir:           c.layersDir,
		launcherPath:        c.launcherPath,
		projectMetadataPath: c.projectMetdataPath,
		runImageRef:         c.runImageRef,
		analyzedMD:          *analyzedMD,
		cacheStore:          cacheStore,
		useDaemon:           c.useDaemon,
		uid:                 c.uid,
		gid:                 c.gid,
		processType:         c.processType,
	})
}
