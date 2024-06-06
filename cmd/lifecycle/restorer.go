package main

import (
	"errors"
	"fmt"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	"github.com/buildpacks/lifecycle/priv"
)

type restoreCmd struct {
	*platform.Platform

	docker   client.CommonAPIClient // construct if necessary before dropping privileges
	keychain authn.Keychain         // construct if necessary before dropping privileges
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (r *restoreCmd) DefineFlags() {
	if r.PlatformAPI.AtLeast("0.13") {
		cli.FlagInsecureRegistries(&r.InsecureRegistries)
	}
	if r.PlatformAPI.AtLeast("0.12") {
		cli.FlagUseDaemon(&r.UseDaemon)
		cli.FlagGeneratedDir(&r.GeneratedDir)
		cli.FlagUseLayout(&r.UseLayout)
		cli.FlagLayoutDir(&r.LayoutDir)
	}
	if r.PlatformAPI.AtLeast("0.10") {
		cli.FlagBuildImage(&r.BuildImageRef)
	}
	cli.FlagAnalyzedPath(&r.AnalyzedPath)
	cli.FlagCacheDir(&r.CacheDir)
	cli.FlagCacheImage(&r.CacheImageRef)
	cli.FlagGID(&r.GID)
	cli.FlagGroupPath(&r.GroupPath)
	cli.FlagLayersDir(&r.LayersDir)
	cli.FlagLogLevel(&r.LogLevel)
	cli.FlagNoColor(&r.NoColor)
	cli.FlagSkipLayers(&r.SkipLayers)
	cli.FlagUID(&r.UID)
}

// Args validates arguments and flags, and fills in default values.
func (r *restoreCmd) Args(nargs int, _ []string) error {
	if nargs > 0 {
		return cmd.FailErrCode(errors.New("received unexpected Args"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	if err := platform.ResolveInputs(platform.Restore, r.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	return nil
}

func (r *restoreCmd) Privileges() error {
	var err error
	r.keychain, err = auth.DefaultKeychain(r.RegistryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}
	if r.UseDaemon {
		var err error
		r.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err = priv.EnsureOwner(r.UID, r.GID, r.LayersDir, r.CacheDir, r.KanikoDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err = priv.RunAs(r.UID, r.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", r.UID, r.GID))
	}
	return nil
}

func (r *restoreCmd) Exec() error {
	factory := phase.NewConnectedFactory(
		r.PlatformAPI,
		&cmd.BuildpackAPIVerifier{},
		NewCacheHandler(r.keychain),
		files.Handler,
		image.NewHandler(r.docker, r.keychain, r.LayoutDir, r.UseLayout, r.InsecureRegistries),
		image.NewRegistryHandler(r.keychain, r.InsecureRegistries),
	)
	restorer, err := factory.NewRestorer(r.Inputs(), cmd.DefaultLogger, buildpack.Group{})
	if err != nil {
		return unwrapErrorFailWithMessage(err, "initialize restorer")
	}
	if err = restorer.RestoreAnalyzed(); err != nil {
		return cmd.FailErrCode(err, r.CodeFor(platform.RestoreError), "restore")
	}
	if err = files.Handler.WriteAnalyzed(r.AnalyzedPath, &restorer.AnalyzedMD, cmd.DefaultLogger); err != nil {
		return cmd.FailErr(err, "write analyzed metadata")
	}
	if err = restorer.RestoreCache(); err != nil {
		return cmd.FailErrCode(err, r.CodeFor(platform.RestoreError), "restore")
	}
	return nil
}
