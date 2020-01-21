package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
)

type rebaseFlags struct {
	imageNames  []string
	runImageRef string
	useDaemon   bool
	useHelpers  bool
}

func parseRebaseFlags() (rebaseFlags, error) {
	f := rebaseFlags{}
	cmd.FlagRunImage(&f.runImageRef)
	cmd.FlagUseDaemon(&f.useDaemon)
	cmd.FlagUseCredHelpers(&f.useHelpers)

	flag.Parse()
	commonFlags()

	f.imageNames = flag.Args()
	if len(f.imageNames) == 0 {
		return rebaseFlags{}, cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
	}
	return f, nil
}

func rebase(f rebaseFlags) error {
	if f.useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), f.imageNames...); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	rebaser := &lifecycle.Rebaser{
		Logger: cmd.Logger,
	}

	registry, err := image.EnsureSingleRegistry(f.imageNames...)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse arguments")
	}

	appImage, err := initImage(f.imageNames[0], f.useDaemon)
	if err != nil || !appImage.Found() {
		return cmd.FailErr(err, "access image to rebase")
	}

	var md lifecycle.LayersMetadata
	if err := lifecycle.DecodeLabel(appImage, lifecycle.LayerMetadataLabel, &md); err != nil {
		return err
	}

	if f.runImageRef == "" {
		if md.Stack.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-image is required when there is no stack metadata available"), cmd.CodeInvalidArgs, "parse arguments")
		}
		f.runImageRef, err = md.Stack.BestRunImageMirror(registry)
		if err != nil {
			return err
		}
	}

	if f.useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), f.imageNames[0], f.runImageRef); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	newBaseImage, err := initImage(f.runImageRef, f.useDaemon)
	if err != nil || !newBaseImage.Found() {
		return cmd.FailErr(err, "access run image")
	}

	if err := rebaser.Rebase(appImage, newBaseImage, f.imageNames[1:]); err != nil {
		if _, ok := err.(*imgutil.SaveError); ok {
			return cmd.FailErrCode(err, cmd.CodeFailedSave, "export")
		}
		return cmd.FailErr(err, "rebase")
	}

	return nil
}
