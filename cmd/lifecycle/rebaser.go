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
	appImage imgutil.Image
	//flags: inputs
	imageNames            []string
	reportPath            string
	runImageRef           string
	deprecatedRunImageRef string
	useDaemon             bool
	uid, gid              int

	platform Platform

	// set if necessary before dropping privileges
	docker   client.CommonAPIClient
	keychain authn.Keychain
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (r *rebaseCmd) DefineFlags() {
	cli.FlagGID(&r.gid)
	cli.FlagReportPath(&r.reportPath)
	cli.FlagRunImage(&r.runImageRef)
	cli.FlagUID(&r.uid)
	cli.FlagUseDaemon(&r.useDaemon)

	cli.DeprecatedFlagRunImage(&r.deprecatedRunImageRef)
}

// Args validates arguments and flags, and fills in default values.
func (r *rebaseCmd) Args(nargs int, args []string) error {
	if nargs == 0 {
		return cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	r.imageNames = args
	if err := image.ValidateDestinationTags(r.useDaemon, r.imageNames...); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "validate image tag(s)")
	}

	if r.deprecatedRunImageRef != "" && r.runImageRef != "" {
		return cmd.FailErrCode(errors.New("supply only one of -run-image or (deprecated) -image"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	if r.deprecatedRunImageRef != "" {
		r.runImageRef = r.deprecatedRunImageRef
	}

	if r.reportPath == platform.PlaceholderReportPath {
		r.reportPath = cli.DefaultReportPath(r.platform.API().String(), "")
	}

	if err := r.setAppImage(); err != nil {
		return cmd.FailErrCode(errors.New(err.Error()), r.platform.CodeFor(platform.RebaseError), "set app image")
	}

	return nil
}

func (r *rebaseCmd) Privileges() error {
	var err error
	r.keychain, err = auth.DefaultKeychain(r.registryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}

	if r.useDaemon {
		var err error
		r.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err := priv.RunAs(r.uid, r.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", r.uid, r.gid))
	}
	return nil
}

func (r *rebaseCmd) Exec() error {
	var err error
	var newBaseImage imgutil.Image
	if r.useDaemon {
		newBaseImage, err = local.NewImage(
			r.runImageRef,
			r.docker,
			local.FromBaseImage(r.runImageRef),
		)
	} else {
		newBaseImage, err = remote.NewImage(
			r.runImageRef,
			r.keychain,
			remote.FromBaseImage(r.runImageRef),
		)
	}
	if err != nil || !newBaseImage.Found() {
		return cmd.FailErr(err, "access run image")
	}

	rebaser := &lifecycle.Rebaser{
		Logger:      cmd.DefaultLogger,
		PlatformAPI: r.platform.API(),
	}
	report, err := rebaser.Rebase(r.appImage, newBaseImage, r.imageNames[1:])
	if err != nil {
		return cmd.FailErrCode(err, r.platform.CodeFor(platform.RebaseError), "rebase")
	}

	if err := encoding.WriteTOML(r.reportPath, &report); err != nil {
		return cmd.FailErrCode(err, r.platform.CodeFor(platform.RebaseError), "write rebase report")
	}
	return nil
}

func (r *rebaseCmd) registryImages() []string {
	registryImages := r.imageNames
	if r.runImageRef != "" {
		registryImages = append(registryImages, r.runImageRef)
	}
	return registryImages
}

func (r *rebaseCmd) setAppImage() error {
	ref, err := name.ParseReference(r.imageNames[0], name.WeakValidation)
	if err != nil {
		return err
	}
	registry := ref.Context().RegistryStr()

	if r.useDaemon {
		r.appImage, err = local.NewImage(
			r.imageNames[0],
			r.docker,
			local.FromBaseImage(r.imageNames[0]),
		)
	} else {
		var keychain authn.Keychain
		keychain, err = auth.DefaultKeychain(r.imageNames[0])
		if err != nil {
			return err
		}
		r.appImage, err = remote.NewImage(
			r.imageNames[0],
			keychain,
			remote.FromBaseImage(r.imageNames[0]),
		)
	}
	if err != nil || !r.appImage.Found() {
		return cmd.FailErr(err, "access image to rebase")
	}

	var md platform.LayersMetadata
	if err := image.DecodeLabel(r.appImage, platform.LayerMetadataLabel, &md); err != nil {
		return err
	}

	if r.runImageRef == "" {
		if md.Stack.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-image is required when there is no stack metadata available"), cmd.CodeForInvalidArgs, "parse arguments")
		}
		r.runImageRef, err = md.Stack.BestRunImageMirror(registry)
		if err != nil {
			return err
		}
	}

	return nil
}
