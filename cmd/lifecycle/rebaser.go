package main

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
)

type rebaseCmd struct {
	imageNames  []string
	runImageRef string
	useDaemon   bool
}

func (r *rebaseCmd) Init() {
	cmd.DeprecatedFlagRunImage(&r.runImageRef)
	cmd.FlagUseDaemon(&r.useDaemon)
}

func (r *rebaseCmd) Args(nargs int, args []string) error {
	if nargs == 0 {
		return cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}
	r.imageNames = args
	return nil
}

func (r *rebaseCmd) Exec() error {
	rebaser := &lifecycle.Rebaser{
		Logger: cmd.Logger,
	}

	registry, err := image.EnsureSingleRegistry(r.imageNames...)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse arguments")
	}

	appImage, err := initImage(r.imageNames[0], r.useDaemon)
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

	newBaseImage, err := initImage(r.runImageRef, r.useDaemon)
	if err != nil || !newBaseImage.Found() {
		return cmd.FailErr(err, "access run image")
	}

	if err := rebaser.Rebase(appImage, newBaseImage, r.imageNames[1:]); err != nil {
		if _, ok := err.(*imgutil.SaveError); ok {
			return cmd.FailErrCode(err, cmd.CodeFailedSave, "export")
		}
		return cmd.FailErr(err, "rebase")
	}

	return nil
}
