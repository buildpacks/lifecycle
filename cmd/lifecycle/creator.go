package main

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/str"
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
	reportPath          string
	runImageRef         string
	stackPath           string
	targetRegistry      string
	uid, gid            int
	skipRestore         bool
	useDaemon           bool

	additionalTags str.Slice
	docker         client.CommonAPIClient // construct if necessary before dropping privileges
	keychain       authn.Keychain
	platform       Platform
	stackMD        platform.StackMetadata
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
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

// Args validates arguments and flags, and fills in default values.
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
		c.projectMetadataPath = cmd.DefaultProjectMetadataPath(c.platform.API().String(), c.layersDir)
	}

	if c.reportPath == cmd.PlaceholderReportPath {
		c.reportPath = cmd.DefaultReportPath(c.platform.API().String(), c.layersDir)
	}

	if c.orderPath == cmd.PlaceholderOrderPath {
		c.orderPath = cmd.DefaultOrderPath(c.platform.API().String(), c.layersDir)
	}

	var err error
	c.stackMD, err = readStack(c.stackPath)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse stack metadata")
	}

	c.targetRegistry, err = parseRegistry(c.outputImageRef)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse target registry")
	}

	if err := c.populateRunImage(); err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "populate run image")
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
	if c.platformAPIVersionGreaterThan06() {
		if err := image.VerifyRegistryAccess(c, c.keychain); err != nil {
			return cmd.FailErr(err)
		}
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

	dirStore, err := platform.NewDirStore(c.buildpacksDir, "")
	if err != nil {
		return err
	}

	var (
		analyzedMD platform.AnalyzedMetadata
		group      buildpack.Group
		plan       platform.BuildPlan
	)
	if c.platform.API().AtLeast("0.7") {
		analyzerFactory := lifecycle.NewAnalyzerFactory(
			c.platform.API(),
			&cmd.APIVerifier{},
			NewCacheHandler(c.keychain),
			lifecycle.NewConfigHandler(),
			NewImageHandler(c.docker, c.keychain),
			NewRegistryHandler(c.keychain),
		)
		analyzer, err := analyzerFactory.NewAnalyzer(
			c.additionalTags,
			c.cacheImageRef,
			c.launchCacheDir,
			c.layersDir,
			"",
			buildpack.Group{},
			"",
			c.outputImageRef,
			c.previousImageRef,
			c.runImageRef,
			c.skipRestore,
			cmd.DefaultLogger,
		)
		if err != nil {
			return cmd.FailErr(err, "initialize analyzer")
		}
		analyzedMD, err = analyzer.Analyze()
		if err != nil {
			return err
		}

		cmd.DefaultLogger.Phase("DETECTING")
		detectorFactory := lifecycle.NewDetectorFactory(
			c.platform.API(),
			&cmd.APIVerifier{},
			lifecycle.NewConfigHandler(),
			dirStore,
		)
		detector, err := detectorFactory.NewDetector(c.appDir, c.orderPath, c.platformDir, cmd.DefaultLogger)
		if err != nil {
			return cmd.FailErr(err, "initialize detector")
		}
		group, plan, err = doDetect(detector, c.platform)
		if err != nil {
			return err
		}
	} else {
		cmd.DefaultLogger.Phase("DETECTING")
		detectorFactory := lifecycle.NewDetectorFactory(
			c.platform.API(),
			&cmd.APIVerifier{},
			lifecycle.NewConfigHandler(),
			dirStore,
		)
		detector, err := detectorFactory.NewDetector(c.appDir, c.orderPath, c.platformDir, cmd.DefaultLogger)
		if err != nil {
			return cmd.FailErr(err, "initialize detector")
		}
		group, plan, err = doDetect(detector, c.platform)
		if err != nil {
			return err
		}

		cmd.DefaultLogger.Phase("ANALYZING")
		analyzerFactory := lifecycle.NewAnalyzerFactory(
			c.platform.API(),
			&cmd.APIVerifier{},
			NewCacheHandler(c.keychain),
			lifecycle.NewConfigHandler(),
			NewImageHandler(c.docker, c.keychain),
			NewRegistryHandler(c.keychain),
		)
		analyzer, err := analyzerFactory.NewAnalyzer(
			c.additionalTags,
			c.cacheImageRef,
			c.launchCacheDir,
			c.layersDir,
			c.cacheDir,
			group,
			"",
			c.outputImageRef,
			c.previousImageRef,
			c.runImageRef,
			c.skipRestore,
			cmd.DefaultLogger,
		)
		if err != nil {
			return cmd.FailErr(err, "initialize analyzer")
		}
		analyzedMD, err = analyzer.Analyze()
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

	// send pings to docker daemon while BUILDING to prevent connection closure
	stopPinging := startPinging(c.docker)
	cmd.DefaultLogger.Phase("BUILDING")
	err = buildArgs{
		buildpacksDir: c.buildpacksDir,
		layersDir:     c.layersDir,
		appDir:        c.appDir,
		platform:      c.platform,
		platformDir:   c.platformDir,
	}.build(group, plan)
	stopPinging()

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
		reportPath:          c.reportPath,
		runImageRef:         c.runImageRef,
		stackMD:             c.stackMD,
		stackPath:           c.stackPath,
		targetRegistry:      c.targetRegistry,
		uid:                 c.uid,
		useDaemon:           c.useDaemon,
	}.export(group, cacheStore, analyzedMD)
}

func (c *createCmd) registryImages() []string {
	var registryImages []string
	registryImages = append(registryImages, c.ReadableRegistryImages()...)
	return append(registryImages, c.WriteableRegistryImages()...)
}

func (c *createCmd) platformAPIVersionGreaterThan06() bool {
	return c.platform.API().AtLeast("0.7")
}

func (c *createCmd) ReadableRegistryImages() []string {
	var readableImages []string
	if !c.useDaemon {
		readableImages = appendNotEmpty(readableImages, c.previousImageRef, c.runImageRef)
	}
	return readableImages
}

func (c *createCmd) WriteableRegistryImages() []string {
	var writeableImages []string
	writeableImages = appendNotEmpty(writeableImages, c.cacheImageRef)
	if !c.useDaemon {
		writeableImages = appendNotEmpty(writeableImages, c.outputImageRef)
		writeableImages = appendNotEmpty(writeableImages, c.additionalTags...)
	}
	return writeableImages
}

func (c *createCmd) populateRunImage() error {
	if c.runImageRef != "" {
		return nil
	}

	var err error
	c.runImageRef, err = c.stackMD.BestRunImageMirror(c.targetRegistry)
	if err != nil {
		return errors.New("-run-image is required when there is no stack metadata available")
	}
	return nil
}

func startPinging(docker client.CommonAPIClient) (stopPinging func()) {
	pingCtx, cancelPing := context.WithCancel(context.Background())
	pingDoneChan := make(chan struct{})
	go func() {
		defer func() { close(pingDoneChan) }()

		if docker == nil {
			return
		}
		for {
			select {
			case <-time.After(time.Millisecond * 500):
				_, err := docker.Ping(pingCtx)
				if err != nil && !errors.Is(err, context.Canceled) {
					cmd.DefaultLogger.Warnf("ping error: %v", err)
				}
			case <-pingCtx.Done():
				return
			}
		}
	}()

	return func() {
		cancelPing()
		<-pingDoneChan
	}
}
