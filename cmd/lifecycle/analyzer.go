package main

import (
	"fmt"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	platform *platform.Platform
	platform.AnalyzeInputs

	docker   client.CommonAPIClient // construct if necessary before dropping privileges
	keychain authn.Keychain         // construct if necessary before dropping privileges
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (a *analyzeCmd) DefineFlags() {
	switch {
	case a.platform.API().AtLeast("0.9"):
		cli.FlagAnalyzedPath(&a.AnalyzedPath)
		cli.FlagCacheImage(&a.CacheImageRef)
		cli.FlagGID(&a.GID)
		cli.FlagLaunchCacheDir(&a.LaunchCacheDir)
		cli.FlagLayersDir(&a.LayersDir)
		cli.FlagPreviousImage(&a.PreviousImageRef)
		cli.FlagRunImage(&a.RunImageRef)
		cli.FlagSkipLayers(&a.SkipLayers)
		cli.FlagStackPath(&a.StackPath)
		cli.FlagTags(&a.AdditionalTags)
		cli.FlagUID(&a.UID)
		cli.FlagUseDaemon(&a.UseDaemon)
	case a.platform.API().AtLeast("0.7"):
		cli.FlagAnalyzedPath(&a.AnalyzedPath)
		cli.FlagCacheImage(&a.CacheImageRef)
		cli.FlagGID(&a.GID)
		cli.FlagLayersDir(&a.LayersDir)
		cli.FlagPreviousImage(&a.PreviousImageRef)
		cli.FlagRunImage(&a.RunImageRef)
		cli.FlagStackPath(&a.StackPath)
		cli.FlagTags(&a.AdditionalTags)
		cli.FlagUID(&a.UID)
		cli.FlagUseDaemon(&a.UseDaemon)
	default:
		cli.FlagAnalyzedPath(&a.AnalyzedPath)
		cli.FlagCacheDir(&a.LegacyCacheDir)
		cli.FlagCacheImage(&a.CacheImageRef)
		cli.FlagGID(&a.GID)
		cli.FlagGroupPath(&a.LegacyGroupPath)
		cli.FlagLayersDir(&a.LayersDir)
		cli.FlagSkipLayers(&a.SkipLayers)
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
	a.AnalyzeInputs.OutputImageRef = args[0]

	var err error
	a.AnalyzeInputs, err = a.platform.ResolveAnalyze(a.AnalyzeInputs, cmd.DefaultLogger)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	return nil
}

// Privileges validates the needed privileges
func (a *analyzeCmd) Privileges() error {
	var err error
	a.keychain, err = auth.DefaultKeychain(a.RegistryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}
	if a.UseDaemon {
		var err error
		a.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err := priv.EnsureOwner(a.UID, a.GID, a.LayersDir, a.LegacyCacheDir, a.LaunchCacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(a.UID, a.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", a.UID, a.GID))
	}
	return nil
}

// Exec executes the command
func (a *analyzeCmd) Exec() error {
	factory := lifecycle.NewAnalyzerFactory(
		a.platform.API(),
		&cmd.BuildpackAPIVerifier{},
		NewCacheHandler(a.keychain),
		lifecycle.NewConfigHandler(),
		NewImageHandler(a.docker, a.keychain),
		NewRegistryHandler(a.keychain),
	)
	analyzer, err := factory.NewAnalyzer(
		a.AdditionalTags,
		a.CacheImageRef,
		a.LaunchCacheDir,
		a.LayersDir,
		a.LegacyCacheDir,
		buildpack.Group{},
		a.LegacyGroupPath,
		a.OutputImageRef,
		a.PreviousImageRef,
		a.RunImageRef,
		a.SkipLayers,
		cmd.DefaultLogger,
	)
	if err != nil {
		return unwrapErrorFailWithMessage(err, "initialize analyzer")
	}

	analyzedMD, err := analyzer.Analyze()
	if err != nil {
		return cmd.FailErrCode(err, a.platform.CodeFor(platform.AnalyzeError), "analyze")
	}

	if err = encoding.WriteTOML(a.AnalyzedPath, analyzedMD); err != nil {
		return cmd.FailErr(err, "write analyzed")
	}
	return nil
}
