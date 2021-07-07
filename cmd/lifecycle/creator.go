package main

import (
	"fmt"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type createCmd struct {
	//flags: inputs
	appDir              string
	buildpacksDir       string
	cacheDir            string
	cacheImageRef       string
	launchCacheDir      string
	launcherPath        string
	layersDir           string
	orderPath           string
	outputImageRef      string
	platformDir         string
	previousImageRef    string
	processType         string
	projectMetadataPath string
	registry            string
	reportPath          string
	runImageRef         string
	stackPath           string
	uid, gid            int
	skipRestore         bool
	useDaemon           bool

	additionalTags cmd.StringSlice
	docker         client.CommonAPIClient // construct if necessary before dropping privileges
	keychain       authn.Keychain
	platform       cmd.Platform
	stackMD        platform.StackMetadata
}

func (c *createCmd) DefineFlags() {
	cmd.FlagAppDir(&c.appDir)
	cmd.FlagBuildpacksDir(&c.buildpacksDir)
	cmd.FlagCacheDir(&c.cacheDir)
	cmd.FlagCacheImage(&c.cacheImageRef)
	cmd.FlagGID(&c.gid)
	cmd.FlagLaunchCacheDir(&c.launchCacheDir)
	cmd.FlagLauncherPath(&c.launcherPath)
	cmd.FlagLayersDir(&c.layersDir)
	cmd.FlagOrderPath(&c.orderPath)
	cmd.FlagPlatformDir(&c.platformDir)
	cmd.FlagPreviousImage(&c.previousImageRef)
	cmd.FlagReportPath(&c.reportPath)
	cmd.FlagRunImage(&c.runImageRef)
	cmd.FlagSkipRestore(&c.skipRestore)
	cmd.FlagStackPath(&c.stackPath)
	cmd.FlagUID(&c.uid)
	cmd.FlagUseDaemon(&c.useDaemon)
	cmd.FlagTags(&c.additionalTags)
	cmd.FlagProjectMetadataPath(&c.projectMetadataPath)
	cmd.FlagProcessType(&c.processType)
}

func (c *createCmd) Args(nargs int, args []string) error {
	if nargs != 1 {
		return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeInvalidArgs, "parse arguments")
	}

	c.outputImageRef = args[0]
	if c.launchCacheDir != "" && !c.useDaemon {
		cmd.DefaultLogger.Warn("Ignoring -launch-cache, only intended for use with -daemon")
		c.launchCacheDir = ""
	}

	if c.cacheImageRef == "" && c.cacheDir == "" {
		cmd.DefaultLogger.Warn("Not restoring or caching layer data, no cache flag specified.")
	}

	if c.previousImageRef == "" {
		c.previousImageRef = c.outputImageRef
	}

	if err := image.ValidateDestinationTags(c.useDaemon, append(c.additionalTags, c.outputImageRef)...); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "validate image tag(s)")
	}

	if c.projectMetadataPath == cmd.PlaceholderProjectMetadataPath {
		c.projectMetadataPath = cmd.DefaultProjectMetadataPath(c.platform.API(), c.layersDir)
	}

	if c.reportPath == cmd.PlaceholderReportPath {
		c.reportPath = cmd.DefaultReportPath(c.platform.API(), c.layersDir)
	}

	if c.orderPath == cmd.PlaceholderOrderPath {
		c.orderPath = cmd.DefaultOrderPath(c.platform.API(), c.layersDir)
	}

	var err error
	c.stackMD, c.runImageRef, c.registry, err = resolveStack(c.outputImageRef, c.stackPath, c.runImageRef)
	if err != nil {
		return err
	}

	if c.runImageRef == "" {
		return cmd.FailErrCode(
			errors.New("-run-image is required when there is no stack metadata available"),
			cmd.CodeInvalidArgs,
			"parse arguments",
		)
	}

	return nil
}

func (c *createCmd) Privileges() error {
	var err error
	c.keychain, err = auth.DefaultKeychain(c.registryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}

	if c.useDaemon {
		var err error
		c.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err := image.VerifyRegistryAccess(c, c.keychain); err != nil {
		return cmd.FailErr(err)
	}
	if err := priv.EnsureOwner(c.uid, c.gid, c.cacheDir, c.launchCacheDir, c.layersDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(c.uid, c.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", c.uid, c.gid))
	}
	if err := priv.SetEnvironmentForUser(c.uid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("set environment for user %d", c.uid))
	}
	return nil
}

func (c *createCmd) Exec() error {
	cacheStore, err := initCache(c.cacheImageRef, c.cacheDir, c.keychain)
	if err != nil {
		return err
	}

	var (
		analyzedMD platform.AnalyzedMetadata
		group      buildpack.Group
		plan       platform.BuildPlan
	)
	if api.MustParse(c.platform.API()).Compare(api.MustParse("0.7")) >= 0 {
		cmd.DefaultLogger.Phase("ANALYZING")
		analyzedMD, err = analyzeArgs{
			additionalTags:   c.additionalTags,
			cacheImageRef:    c.cacheImageRef,
			docker:           c.docker,
			keychain:         c.keychain,
			layersDir:        c.layersDir,
			outputImageRef:   c.outputImageRef,
			platform:         c.platform,
			previousImageRef: c.previousImageRef,
			runImageRef:      c.runImageRef,
			useDaemon:        c.useDaemon,
		}.analyze()
		if err != nil {
			return err
		}

		cmd.DefaultLogger.Phase("DETECTING")
		group, plan, err = detectArgs{
			buildpacksDir: c.buildpacksDir,
			appDir:        c.appDir,
			layersDir:     c.layersDir,
			platform:      c.platform,
			platformDir:   c.platformDir,
			orderPath:     c.orderPath,
		}.detect()
		if err != nil {
			return err
		}
	} else {
		cmd.DefaultLogger.Phase("DETECTING")
		group, plan, err = detectArgs{
			buildpacksDir: c.buildpacksDir,
			appDir:        c.appDir,
			layersDir:     c.layersDir,
			platform:      c.platform,
			platformDir:   c.platformDir,
			orderPath:     c.orderPath,
		}.detect()
		if err != nil {
			return err
		}

		cmd.DefaultLogger.Phase("ANALYZING")
		analyzedMD, err = analyzeArgs{
			docker:           c.docker,
			keychain:         c.keychain,
			layersDir:        c.layersDir,
			previousImageRef: c.previousImageRef,
			platform:         c.platform,
			useDaemon:        c.useDaemon,
			platform06: analyzeArgsPlatform06{
				skipLayers: c.skipRestore,
				group:      group,
				cache:      cacheStore,
			},
		}.analyze()
		if err != nil {
			return err
		}
	}

	if !c.skipRestore {
		cmd.DefaultLogger.Phase("RESTORING")
		err := restoreArgs{
			keychain:   c.keychain,
			layersDir:  c.layersDir,
			platform:   c.platform,
			skipLayers: c.skipRestore,
		}.restore(analyzedMD.Metadata, group, cacheStore)
		if err != nil {
			return err
		}
	}

	cmd.DefaultLogger.Phase("BUILDING")
	err = buildArgs{
		buildpacksDir: c.buildpacksDir,
		layersDir:     c.layersDir,
		appDir:        c.appDir,
		platform:      c.platform,
		platformDir:   c.platformDir,
	}.build(group, plan)
	if err != nil {
		return err
	}

	cmd.DefaultLogger.Phase("EXPORTING")
	return exportArgs{
		appDir:              c.appDir,
		docker:              c.docker,
		gid:                 c.gid,
		imageNames:          append([]string{c.outputImageRef}, c.additionalTags...),
		keychain:            c.keychain,
		launchCacheDir:      c.launchCacheDir,
		launcherPath:        c.launcherPath,
		layersDir:           c.layersDir,
		platform:            c.platform,
		processType:         c.processType,
		projectMetadataPath: c.projectMetadataPath,
		registry:            c.registry,
		reportPath:          c.reportPath,
		runImageRef:         c.runImageRef,
		stackMD:             c.stackMD,
		stackPath:           c.stackPath,
		uid:                 c.uid,
		useDaemon:           c.useDaemon,
	}.export(group, cacheStore, analyzedMD)
}

func (c *createCmd) registryImages() []string {
	var registryImages []string
	if c.cacheImageRef != "" {
		registryImages = append(registryImages, c.cacheImageRef)
	}
	if !c.useDaemon {
		registryImages = append(registryImages, append([]string{c.outputImageRef}, c.additionalTags...)...)
		registryImages = append(registryImages, c.runImageRef, c.previousImageRef)
	}
	return registryImages
}

func (c *createCmd) platformAPIVersionGreaterThan06() bool {
	return api.MustParse(c.platform.API()).Compare(api.MustParse("0.7")) >= 0
}

func (c *createCmd) ReadableImages() []string {
	var readableImages []string
	if c.platformAPIVersionGreaterThan06() {
		if !c.useDaemon {
			readableImages = append(readableImages, c.previousImageRef, c.runImageRef)
		}
	}
	return readableImages
}
func (c *createCmd) WriteableImages() []string {
	var writeableImages []string
	if c.platformAPIVersionGreaterThan06() {
		if !c.useDaemon {
			writeableImages = append(writeableImages, append([]string{c.cacheImageRef}, c.additionalTags...)...)
		}
	}
	return writeableImages
}
