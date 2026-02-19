package main

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/moby/moby/client"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	*platform.Platform

	docker   client.APIClient // construct if necessary before dropping privileges
	keychain authn.Keychain   // construct if necessary before dropping privileges
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (a *analyzeCmd) DefineFlags() {
	switch {
	case a.PlatformAPI.AtLeast("0.13"):
		cli.FlagInsecureRegistries(&a.InsecureRegistries)
		fallthrough
	case a.PlatformAPI.AtLeast("0.12"):
		cli.FlagLayoutDir(&a.LayoutDir)
		cli.FlagRunPath(&a.RunPath)
		cli.FlagUseLayout(&a.UseLayout)
		fallthrough
	case a.PlatformAPI.AtLeast("0.9"):
		cli.FlagLaunchCacheDir(&a.LaunchCacheDir)
		cli.FlagSkipLayers(&a.SkipLayers)
		fallthrough
	default:
		cli.FlagAnalyzedPath(&a.AnalyzedPath)
		cli.FlagCacheImage(&a.CacheImageRef)
		cli.FlagGID(&a.GID)
		cli.FlagLayersDir(&a.LayersDir)
		cli.FlagLogLevel(&a.LogLevel)
		cli.FlagNoColor(&a.NoColor)
		cli.FlagPreviousImage(&a.PreviousImageRef)
		cli.FlagRunImage(&a.RunImageRef)
		cli.FlagTags(&a.AdditionalTags)
		cli.FlagUID(&a.UID)
		cli.FlagUseDaemon(&a.UseDaemon)
	}
	// deprecated
	if a.PlatformAPI.LessThan("0.12") {
		cli.FlagStackPath(&a.StackPath)
	}
}

// Args validates arguments and flags, and fills in default values.
func (a *analyzeCmd) Args(nargs int, args []string) error {
	if nargs != 1 {
		err := fmt.Errorf("received %d arguments, but expected 1", nargs)
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "parse arguments")
	}
	a.LifecycleInputs.OutputImageRef = args[0]
	if err := platform.ResolveInputs(platform.Analyze, a.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	if a.UseLayout {
		if err := platform.GuardExperimental(platform.LayoutFormat, cmd.DefaultLogger); err != nil {
			return err
		}
	}
	return nil
}

// Privileges validates the needed privileges.
func (a *analyzeCmd) Privileges() error {
	var err error
	a.keychain, err = auth.DefaultKeychain(a.RegistryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}
	if a.UseDaemon {
		a.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err = priv.EnsureOwner(a.UID, a.GID, a.LayersDir, a.CacheDir, a.LaunchCacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err = priv.RunAs(a.UID, a.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", a.UID, a.GID))
	}
	return nil
}

// Exec executes the command.
func (a *analyzeCmd) Exec() error {
	factory := phase.NewConnectedFactory(
		a.PlatformAPI,
		&cmd.BuildpackAPIVerifier{},
		NewCacheHandler(a.keychain, a.InsecureRegistries),
		files.Handler,
		image.NewHandler(a.docker, a.keychain, a.LayoutDir, a.UseLayout, a.InsecureRegistries),
		image.NewRegistryHandler(a.keychain, a.InsecureRegistries),
	)
	analyzer, err := factory.NewAnalyzer(a.Inputs(), cmd.DefaultLogger)
	if err != nil {
		return unwrapErrorFailWithMessage(err, "initialize analyzer")
	}
	analyzedMD, err := analyzer.Analyze()
	if err != nil {
		return cmd.FailErrCode(err, a.CodeFor(platform.AnalyzeError), "analyze")
	}
	return files.Handler.WriteAnalyzed(a.AnalyzedPath, &analyzedMD, cmd.DefaultLogger)
}
