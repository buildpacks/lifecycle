package main

import (
	"fmt"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
	newplat "github.com/buildpacks/lifecycle/cmd/lifecycle/platform"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	platform Platform

	newplat.AnalyzeInputs
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
func (a *analyzeCmd) Args(_ int, args []string) error {
	resolver := &newplat.AnalyzeInputsResolver{PlatformAPI: a.platform.API()}
	resolvedInputs, err := resolver.Resolve(a.AnalyzeInputs, args, cmd.DefaultLogger)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "resolve inputs")
	}
	a.AnalyzeInputs = resolvedInputs
	return nil
}

func (a *analyzeCmd) Privileges() error {
	var err error
	a.keychain, err = auth.DefaultKeychain(a.registryImages()...)
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
	if a.platform.API().AtLeast("0.7") {
		if err := image.VerifyRegistryAccess(a, a.keychain); err != nil {
			return cmd.FailErr(err)
		}
	}
	if err := priv.EnsureOwner(a.UID, a.GID, a.LayersDir, a.LegacyCacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(a.UID, a.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", a.UID, a.GID))
	}
	return nil
}

func (a *analyzeCmd) registryImages() []string {
	var registryImages []string
	registryImages = append(registryImages, a.ReadableRegistryImages()...)
	return append(registryImages, a.WriteableRegistryImages()...)
}

func (a *analyzeCmd) Exec() error {
	factory := newplat.NewAnalyzerFactory(a.platform.API(), a.docker, a.keychain)
	analyzer, err := factory.NewAnalyzer(newplat.AnalyzerOpts{
		CacheImageRef:    a.CacheImageRef,
		LaunchCacheDir:   a.LaunchCacheDir,
		LayersDir:        a.LayersDir,
		LegacyCacheDir:   a.LegacyCacheDir,
		LegacyGroupPath:  a.LegacyGroupPath,
		PreviousImageRef: a.PreviousImageRef,
		RunImageRef:      a.RunImageRef,
		SkipLayers:       a.SkipLayers,
	}, cmd.DefaultLogger)
	if err != nil {
		if err, ok := err.(*cmd.ErrorFail); ok {
			return err
		}
		return errors.Wrap(err, "initializing analyzer")
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

func (a *analyzeCmd) ReadableRegistryImages() []string {
	var readableImages []string
	if !a.UseDaemon {
		readableImages = appendNotEmpty(readableImages, a.PreviousImageRef, a.RunImageRef)
	}
	return readableImages
}

func (a *analyzeCmd) WriteableRegistryImages() []string {
	var writeableImages []string
	writeableImages = appendNotEmpty(writeableImages, a.CacheImageRef)
	if !a.UseDaemon {
		writeableImages = appendNotEmpty(writeableImages, a.OutputImageRef)
		writeableImages = appendNotEmpty(writeableImages, a.AdditionalTags...)
	}
	return writeableImages
}
