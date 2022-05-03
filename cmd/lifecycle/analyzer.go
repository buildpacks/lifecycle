package main

import (
	"fmt"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	platform Platform
	platform.AnalyzeInputs

	docker   client.CommonAPIClient // construct if necessary before dropping privileges
	keychain authn.Keychain         // construct if necessary before dropping privileges
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (a *analyzeCmd) DefineFlags() {
	switch {
	case a.platform.API().AtLeast("0.9"):
		cmd.FlagAnalyzedPath(&a.AnalyzedPath)
		cmd.FlagCacheImage(&a.CacheImageRef)
		cmd.FlagGID(&a.GID)
		cmd.FlagLaunchCacheDir(&a.LaunchCacheDir)
		cmd.FlagLayersDir(&a.LayersDir)
		cmd.FlagPreviousImage(&a.PreviousImageRef)
		cmd.FlagRunImage(&a.RunImageRef)
		cmd.FlagSkipLayers(&a.SkipLayers)
		cmd.FlagStackPath(&a.StackPath)
		cmd.FlagTags(&a.AdditionalTags)
		cmd.FlagUID(&a.UID)
		cmd.FlagUseDaemon(&a.UseDaemon)
	case a.platform.API().AtLeast("0.7"):
		cmd.FlagAnalyzedPath(&a.AnalyzedPath)
		cmd.FlagCacheImage(&a.CacheImageRef)
		cmd.FlagGID(&a.GID)
		cmd.FlagLayersDir(&a.LayersDir)
		cmd.FlagPreviousImage(&a.PreviousImageRef)
		cmd.FlagRunImage(&a.RunImageRef)
		cmd.FlagStackPath(&a.StackPath)
		cmd.FlagTags(&a.AdditionalTags)
		cmd.FlagUID(&a.UID)
		cmd.FlagUseDaemon(&a.UseDaemon)
	default:
		cmd.FlagAnalyzedPath(&a.AnalyzedPath)
		cmd.FlagCacheDir(&a.LegacyCacheDir)
		cmd.FlagCacheImage(&a.CacheImageRef)
		cmd.FlagGID(&a.GID)
		cmd.FlagGroupPath(&a.LegacyGroupPath)
		cmd.FlagLayersDir(&a.LayersDir)
		cmd.FlagSkipLayers(&a.SkipLayers)
		cmd.FlagUID(&a.UID)
		cmd.FlagUseDaemon(&a.UseDaemon)
	}
}

// Args validates arguments and flags, and fills in default values.
func (a *analyzeCmd) Args(nargs int, args []string) error {
	if nargs != 1 {
		err := fmt.Errorf("received %d arguments, but expected 1", nargs)
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse arguments")
	}
	a.AnalyzeInputs.OutputImageRef = args[0]

	var err error
	a.AnalyzeInputs, err = a.platform.ResolveAnalyze(a.AnalyzeInputs, cmd.DefaultLogger)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "resolve inputs")
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
		NewCacheHandler(a.keychain),
		lifecycle.NewConfigHandler(&cmd.APIVerifier{}),
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
		return err
	}

	analyzedMD, err := analyzer.Analyze()
	if err != nil {
		return cmd.FailErrCode(err, a.platform.CodeFor(platform.AnalyzeError), "analyzer")
	}

	if err = encoding.WriteTOML(a.AnalyzedPath, analyzedMD); err != nil {
		return errors.Wrap(err, "writing analyzed.toml")
	}
	return nil
}
