package main

import (
	"context"
	"fmt"
	"time"

	"github.com/buildpacks/lifecycle/image"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type createCmd struct {
	*platform.Platform

	docker   client.CommonAPIClient // construct if necessary before dropping privileges
	keychain authn.Keychain         // construct if necessary before dropping privileges
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (c *createCmd) DefineFlags() {
	if c.PlatformAPI.AtLeast("0.12") {
		cli.FlagLayoutDir(&c.LayoutDir)
		cli.FlagUseLayout(&c.UseLayout)
		cli.FlagRunPath(&c.RunPath)
	}
	if c.PlatformAPI.AtLeast("0.11") {
		cli.FlagBuildConfigDir(&c.BuildConfigDir)
		cli.FlagLauncherSBOMDir(&c.LauncherSBOMDir)
	}
	cli.FlagAppDir(&c.AppDir)
	cli.FlagBuildpacksDir(&c.BuildpacksDir)
	cli.FlagCacheDir(&c.CacheDir)
	cli.FlagCacheImage(&c.CacheImageRef)
	cli.FlagGID(&c.GID)
	cli.FlagLaunchCacheDir(&c.LaunchCacheDir)
	cli.FlagLauncherPath(&c.LauncherPath)
	cli.FlagLayersDir(&c.LayersDir)
	cli.FlagOrderPath(&c.OrderPath)
	cli.FlagPlatformDir(&c.PlatformDir)
	cli.FlagPreviousImage(&c.PreviousImageRef)
	cli.FlagProcessType(&c.DefaultProcessType)
	cli.FlagProjectMetadataPath(&c.ProjectMetadataPath)
	cli.FlagReportPath(&c.ReportPath)
	cli.FlagRunImage(&c.RunImageRef)
	cli.FlagSkipRestore(&c.SkipLayers)
	cli.FlagStackPath(&c.StackPath)
	cli.FlagTags(&c.AdditionalTags)
	cli.FlagUID(&c.UID)
	cli.FlagUseDaemon(&c.UseDaemon)
}

// Args validates arguments and flags, and fills in default values.
func (c *createCmd) Args(nargs int, args []string) error {
	if nargs != 1 {
		return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeForInvalidArgs, "parse arguments")
	}
	c.OutputImageRef = args[0]
	if err := platform.ResolveInputs(platform.Create, c.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	if c.UseLayout {
		if err := platform.GuardExperimental(platform.LayoutFormat, cmd.DefaultLogger); err != nil {
			return err
		}
	}
	return nil
}

func (c *createCmd) Privileges() error {
	var err error
	c.keychain, err = auth.DefaultKeychain(c.RegistryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}
	if c.UseDaemon {
		c.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err = priv.EnsureOwner(c.UID, c.GID, c.CacheDir, c.LaunchCacheDir, c.LayersDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err = priv.RunAs(c.UID, c.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", c.UID, c.GID))
	}
	if err = priv.SetEnvironmentForUser(c.UID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("set environment for user %d", c.UID))
	}
	return nil
}

func (c *createCmd) Exec() error {
	cacheStore, err := initCache(c.CacheImageRef, c.CacheDir, c.keychain)
	if err != nil {
		return err
	}
	dirStore := platform.NewDirStore(c.BuildpacksDir, "")
	if err != nil {
		return err
	}

	// Analyze, Detect
	var (
		analyzedMD platform.AnalyzedMetadata
		group      buildpack.Group
		plan       platform.BuildPlan
	)
	if c.PlatformAPI.AtLeast("0.7") {
		cmd.DefaultLogger.Phase("ANALYZING")
		analyzerFactory := lifecycle.NewAnalyzerFactory(
			c.PlatformAPI,
			&cmd.BuildpackAPIVerifier{},
			NewCacheHandler(c.keychain),
			lifecycle.NewConfigHandler(),
			image.NewHandler(c.docker, c.keychain, c.LayoutDir, c.UseLayout),
			NewRegistryHandler(c.keychain),
		)
		analyzer, err := analyzerFactory.NewAnalyzer(
			c.AdditionalTags,
			c.CacheImageRef,
			c.LaunchCacheDir,
			c.LayersDir,
			"",
			buildpack.Group{},
			"",
			c.OutputImageRef,
			c.PreviousImageRef,
			c.RunImageRef,
			c.SkipLayers,
			cmd.DefaultLogger,
		)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize analyzer")
		}
		analyzedMD, err = analyzer.Analyze()
		if err != nil {
			return err
		}

		cmd.DefaultLogger.Phase("DETECTING")
		detectorFactory := lifecycle.NewDetectorFactory(
			c.PlatformAPI,
			&cmd.BuildpackAPIVerifier{},
			lifecycle.NewConfigHandler(),
			dirStore,
		)
		detector, err := detectorFactory.NewDetector(analyzedMD, c.AppDir, c.BuildConfigDir, c.OrderPath, c.PlatformDir, cmd.DefaultLogger)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize detector")
		}
		group, plan, err = doDetect(detector, c.Platform)
		if err != nil {
			return err // pass through error
		}
	} else {
		cmd.DefaultLogger.Phase("DETECTING")
		detectorFactory := lifecycle.NewDetectorFactory(
			c.PlatformAPI,
			&cmd.BuildpackAPIVerifier{},
			lifecycle.NewConfigHandler(),
			dirStore,
		)
		detector, err := detectorFactory.NewDetector(platform.AnalyzedMetadata{}, c.AppDir, c.BuildConfigDir, c.OrderPath, c.PlatformDir, cmd.DefaultLogger)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize detector")
		}
		group, plan, err = doDetect(detector, c.Platform)
		if err != nil {
			return err // pass through error
		}

		cmd.DefaultLogger.Phase("ANALYZING")
		analyzerFactory := lifecycle.NewAnalyzerFactory(
			c.PlatformAPI,
			&cmd.BuildpackAPIVerifier{},
			NewCacheHandler(c.keychain),
			lifecycle.NewConfigHandler(),
			image.NewHandler(c.docker, c.keychain, c.LayoutDir, c.UseLayout),
			NewRegistryHandler(c.keychain),
		)
		analyzer, err := analyzerFactory.NewAnalyzer(
			c.AdditionalTags,
			c.CacheImageRef,
			c.LaunchCacheDir,
			c.LayersDir,
			c.CacheDir,
			group,
			"",
			c.OutputImageRef,
			c.PreviousImageRef,
			c.RunImageRef,
			c.SkipLayers,
			cmd.DefaultLogger,
		)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize analyzer")
		}
		analyzedMD, err = analyzer.Analyze()
		if err != nil {
			return err
		}
	}

	// Restore
	if !c.SkipLayers || c.PlatformAPI.AtLeast("0.10") {
		cmd.DefaultLogger.Phase("RESTORING")
		restoreCmd := &restoreCmd{
			Platform: c.Platform,
			keychain: c.keychain,
		}
		err := restoreCmd.restore(analyzedMD.Metadata, group, cacheStore)
		if err != nil {
			return err
		}
	}

	// Build
	stopPinging := startPinging(c.docker) // send pings to docker daemon while building to prevent connection closure
	cmd.DefaultLogger.Phase("BUILDING")
	buildCmd := &buildCmd{Platform: c.Platform}
	err = buildCmd.build(group, plan, analyzedMD)
	stopPinging()
	if err != nil {
		return err
	}

	// Export
	cmd.DefaultLogger.Phase("EXPORTING")
	exportCmd := &exportCmd{
		Platform: c.Platform,
		docker:   c.docker,
		keychain: c.keychain,
	}
	return exportCmd.export(group, cacheStore, analyzedMD)
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
