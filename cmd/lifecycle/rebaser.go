package main

import (
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/priv"
)

type rebaseCmd struct {
	//flags: inputs
	imageNames            []string
	runImageRef           string
	deprecatedRunImageRef string
	useDaemon             bool
	uid, gid              int

	//set if necessary before dropping privileges
	docker client.CommonAPIClient
}

func (r *rebaseCmd) Init() {
	cmd.DeprecatedFlagRunImage(&r.deprecatedRunImageRef)
	cmd.FlagRunImage(&r.runImageRef)
	cmd.FlagUseDaemon(&r.useDaemon)
	cmd.FlagUID(&r.uid)
	cmd.FlagGID(&r.gid)
}

func (r *rebaseCmd) Args(nargs int, args []string) error {
	if nargs == 0 {
		return cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}
	r.imageNames = args
	if err := image.EnsureSingleRegistry(r.imageNames...); err != nil {
		return cmd.FailErrCode(image.EnsureSingleRegistry(r.imageNames...), cmd.CodeInvalidArgs, "images tags must all have the same registry")
	}

	if r.deprecatedRunImageRef != "" && r.runImageRef != "" {
		return cmd.FailErrCode(errors.New("supply only one of -run-image or (deprecated) -image"), cmd.CodeInvalidArgs, "parse arguments")
	}
	if r.deprecatedRunImageRef != "" {
		r.runImageRef = r.deprecatedRunImageRef
	}
	return nil
}

func (r *rebaseCmd) Privileges() error {
	if r.useDaemon {
		var err error
		r.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err := priv.RunAs(r.uid, r.gid, true); err != nil {
		cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", r.uid, r.gid))
	}
	return nil
}

func (r *rebaseCmd) Exec() error {
	ref, err := name.ParseReference(r.imageNames[0], name.WeakValidation)
	if err != nil {
		return err
	}
	registry := ref.Context().RegistryStr()

	var appImage imgutil.Image
	if r.useDaemon {
		appImage, err = local.NewImage(
			r.imageNames[0],
			r.docker,
			local.FromBaseImage(r.imageNames[0]),
		)
	} else {
		appImage, err = remote.NewImage(
			r.imageNames[0],
			auth.EnvKeychain(cmd.EnvRegistryAuth),
			remote.FromBaseImage(r.imageNames[0]),
		)
	}
	if err != nil || !appImage.Found() {
		return cmd.FailErr(err, "access image to rebase")
	}

	var md lifecycle.LayersMetadata
	if err := lifecycle.DecodeLabel(appImage, lifecycle.LayerMetadataLabel, &md); err != nil {
		return err
	}

	if r.runImageRef == "" {
		if md.Stack.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-image is required when there is no stack metadata available"), cmd.CodeInvalidArgs, "parse arguments")
		}
		r.runImageRef, err = md.Stack.BestRunImageMirror(registry)
		if err != nil {
			return err
		}
	}

	var newBaseImage imgutil.Image
	if r.useDaemon {
		newBaseImage, err = local.NewImage(
			r.imageNames[0],
			r.docker,
			local.FromBaseImage(r.runImageRef),
		)
	} else {
		newBaseImage, err = remote.NewImage(
			r.imageNames[0],
			auth.EnvKeychain(cmd.EnvRegistryAuth),
			remote.FromBaseImage(r.runImageRef),
		)
	}
	if err != nil || !newBaseImage.Found() {
		return cmd.FailErr(err, "access run image")
	}

	rebaser := &lifecycle.Rebaser{
		Logger: cmd.Logger,
	}
	if err := rebaser.Rebase(appImage, newBaseImage, r.imageNames[1:]); err != nil {
		if _, ok := err.(*imgutil.SaveError); ok {
			return cmd.FailErrCode(err, cmd.CodeFailedSave, "rebase")
		}
		return cmd.FailErr(err, "rebase")
	}
	return nil
}
