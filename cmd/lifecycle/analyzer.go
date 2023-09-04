package main

import (
	"fmt"

	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/encoding"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	*platform.Platform

	docker   client.CommonAPIClient // construct if necessary before dropping privileges
	keychain authn.Keychain         // construct if necessary before dropping privileges
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (a *analyzeCmd) DefineFlags() {
	if a.PlatformAPI.LessThan("0.12") {
		cli.FlagStackPath(&a.StackPath)
	}
	switch {
	case a.PlatformAPI.AtLeast("0.12"):
		cli.FlagLayoutDir(&a.LayoutDir)
		cli.FlagUseLayout(&a.UseLayout)
		cli.FlagRunPath(&a.RunPath)
		cli.FlagInsecureRegistries(&a.InsecureRegistries)
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
		cli.FlagPreviousImage(&a.PreviousImageRef)
		cli.FlagRunImage(&a.RunImageRef)
		cli.FlagTags(&a.AdditionalTags)
		cli.FlagUID(&a.UID)
		cli.FlagUseDaemon(&a.UseDaemon)
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
	factory := lifecycle.NewAnalyzerFactory(
		a.PlatformAPI,
		&cmd.BuildpackAPIVerifier{},
		NewCacheHandler(a.keychain),
		lifecycle.NewConfigHandler(),
		image.NewHandler(a.docker, a.keychain, a.LayoutDir, a.UseLayout, a.InsecureRegistries),
		image.NewRegistryHandler(a.keychain, a.InsecureRegistries),
	)
	analyzer, err := factory.NewAnalyzer(a.AdditionalTags, a.CacheImageRef, a.LaunchCacheDir, a.LayersDir, a.OutputImageRef, a.PreviousImageRef, a.RunImageRef, a.SkipLayers, cmd.DefaultLogger)
	if err != nil {
		return unwrapErrorFailWithMessage(err, "initialize analyzer")
	}
	analyzedMD, err := analyzer.Analyze()
	if err != nil {
		return cmd.FailErrCode(err, a.CodeFor(platform.AnalyzeError), "analyze")
	}
	cmd.DefaultLogger.Debugf("Run image info in analyzed metadata is: ")
	cmd.DefaultLogger.Debugf(encoding.ToJSONMaybe(analyzedMD.RunImage))
	if err = encoding.WriteTOML(a.AnalyzedPath, analyzedMD); err != nil {
		return cmd.FailErr(err, "write analyzed")
	}
	return nil
}
