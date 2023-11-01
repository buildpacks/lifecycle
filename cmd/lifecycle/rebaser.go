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

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
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

	if r.PlatformAPI.AtLeast("0.13") {
		cli.FlagInsecureRegistries(&r.InsecureRegistries)
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
	var err error
	if !r.UseDaemon {
		// We may need to read the application image in order to know the run image, so
		// we read the application here so that the keychain will include the run image.
		// We need to construct a keychain for the run image before we drop privileges,
		// in case the credentials will become inaccessible to a non-root user.
		// We cannot read the application image if we're working with a daemon,
		// because the docker client hasn't been constructed yet.
		if err = r.setAppImage(); err != nil {
			return cmd.FailErrCode(errors.New(err.Error()), r.CodeFor(platform.RebaseError), "set app image")
		}
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
	if r.UseDaemon {
		// We may need to read the application image in order to know the run image,
		// but the run image is in a daemon (not a registry), so it is okay
		// to read the application here because the keychain doesn't need to include the run image.
		if err = r.setAppImage(); err != nil {
			return cmd.FailErrCode(errors.New(err.Error()), r.CodeFor(platform.RebaseError), "set app image")
		}
	}
	var newBaseImage imgutil.Image
	if r.UseDaemon {
		newBaseImage, err = local.NewImage(
			r.RunImageRef,
			r.docker,
			local.FromBaseImage(r.RunImageRef),
		)
	} else {
		var opts []remote.ImageOption
		opts = append(opts, append(image.GetInsecureOptions(r.InsecureRegistries), remote.FromBaseImage(r.RunImageRef))...)

		newBaseImage, err = remote.NewImage(
			r.RunImageRef,
			r.keychain,
			opts...,
		)
	}
	if err != nil || !newBaseImage.Found() {
		return cmd.FailErr(err, "access run image")
	}

	rebaser := &phase.Rebaser{
		Logger:      cmd.DefaultLogger,
		PlatformAPI: r.PlatformAPI,
		Force:       r.ForceRebase,
	}
	report, err := rebaser.Rebase(r.appImage, newBaseImage, r.OutputImageRef, r.AdditionalTags)
	if err != nil {
		return cmd.FailErrCode(err, r.CodeFor(platform.RebaseError), "rebase")
	}
	if err = files.Handler.WriteRebaseReport(r.ReportPath, &report); err != nil {
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

		var opts = []remote.ImageOption{
			remote.FromBaseImage(targetImageRef),
		}

		opts = append(opts, image.GetInsecureOptions(r.InsecureRegistries)...)

		r.appImage, err = remote.NewImage(
			targetImageRef,
			keychain,
			opts...,
		)
	}
	if err != nil || !r.appImage.Found() {
		return cmd.FailErr(err, "access image to rebase")
	}

	var md files.LayersMetadata
	if err := image.DecodeLabel(r.appImage, platform.LifecycleMetadataLabel, &md); err != nil {
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
		if md.Stack == nil || md.Stack.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-run-image is required when there is no run image metadata available"), cmd.CodeForInvalidArgs, "parse arguments")
		}

		// for older platforms, we find the best mirror for the run image as this point
		r.RunImageRef, err = platform.BestRunImageMirrorFor(registry, md.Stack.RunImage, r.LifecycleInputs.AccessChecker())
		if err != nil {
			return err
		}
	}

	return nil
}
