package main

import (
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type rebaseCmd struct {
	*platform.Platform

	docker   client.CommonAPIClient // construct if necessary before dropping privileges
	keychain authn.Keychain         // construct if necessary before dropping privileges

	appImage imgutil.Image
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (r *rebaseCmd) DefineFlags() {
	cli.FlagGID(&r.GID)
	cli.FlagReportPath(&r.ReportPath)
	cli.FlagRunImage(&r.RunImageRef)
	cli.FlagUID(&r.UID)
	cli.FlagUseDaemon(&r.UseDaemon)
	cli.DeprecatedFlagRunImage(&r.DeprecatedRunImageRef)

	if r.PlatformAPI.AtLeast("0.11") {
		cli.FlagPreviousImage(&r.PreviousImageRef)
	}

	if r.PlatformAPI.AtLeast("0.12") {
		cli.FlagForceRebase(&r.ForceRebase)
	}
}

// Args validates arguments and flags, and fills in default values.
func (r *rebaseCmd) Args(nargs int, args []string) error {
	if nargs == 0 {
		return cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	r.OutputImageRef = args[0]
	r.AdditionalTags = args[1:]
	if err := platform.ResolveInputs(platform.Rebase, r.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	if err := r.setAppImage(); err != nil {
		return cmd.FailErrCode(errors.New(err.Error()), r.CodeFor(platform.RebaseError), "set app image")
	}
	return nil
}

func (r *rebaseCmd) Privileges() error {
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
	if err := priv.RunAs(r.UID, r.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", r.UID, r.GID))
	}
	return nil
}

func (r *rebaseCmd) Exec() error {
	var err error
	var newBaseImage imgutil.Image
	if r.UseDaemon {
		newBaseImage, err = local.NewImage(
			r.RunImageRef,
			r.docker,
			local.FromBaseImage(r.RunImageRef),
		)
	} else {
		newBaseImage, err = remote.NewImage(
			r.RunImageRef,
			r.keychain,
			remote.FromBaseImage(r.RunImageRef),
		)
	}
	if err != nil || !newBaseImage.Found() {
		return cmd.FailErr(err, "access run image")
	}

	rebaser := &lifecycle.Rebaser{
		Logger:      cmd.DefaultLogger,
		PlatformAPI: r.PlatformAPI,
		Force:       r.ForceRebase,
	}
	report, err := rebaser.Rebase(r.appImage, newBaseImage, r.OutputImageRef, r.AdditionalTags)
	if err != nil {
		return cmd.FailErrCode(err, r.CodeFor(platform.RebaseError), "rebase")
	}

	if err := encoding.WriteTOML(r.ReportPath, &report); err != nil {
		return cmd.FailErrCode(err, r.CodeFor(platform.RebaseError), "write rebase report")
	}
	return nil
}

func (r *rebaseCmd) setAppImage() error {
	var targetImageRef string
	if len(r.PreviousImageRef) > 0 {
		targetImageRef = r.PreviousImageRef
	} else {
		targetImageRef = r.OutputImageRef
	}

	ref, err := name.ParseReference(targetImageRef, name.WeakValidation)
	if err != nil {
		return err
	}
	registry := ref.Context().RegistryStr()

	if r.UseDaemon {
		r.appImage, err = local.NewImage(
			targetImageRef,
			r.docker,
			local.FromBaseImage(targetImageRef),
		)
	} else {
		var keychain authn.Keychain
		keychain, err = auth.DefaultKeychain(targetImageRef)
		if err != nil {
			return err
		}
		r.appImage, err = remote.NewImage(
			targetImageRef,
			keychain,
			remote.FromBaseImage(targetImageRef),
		)
	}
	if err != nil || !r.appImage.Found() {
		return cmd.FailErr(err, "access image to rebase")
	}

	var md platform.LayersMetadata
	if err := image.DecodeLabel(r.appImage, platform.LayerMetadataLabel, &md); err != nil {
		return err
	}

	if r.RunImageRef == "" {
		if r.PlatformAPI.AtLeast("0.12") {
			r.RunImageRef = md.RunImage.Reference
			if r.RunImageRef != "" {
				return nil
			}
		}

		// for backwards compatibility, we need to fallback to the stack metadata
		// fail if there is no run image metadata available from either location
		if md.Stack.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-run-image is required when there is no run image metadata available"), cmd.CodeForInvalidArgs, "parse arguments")
		}

		// for older platforms, we find the best mirror for the run image as this point
		r.RunImageRef, err = md.Stack.BestRunImageMirror(registry)
		if err != nil {
			return err
		}
	}

	return nil
}
