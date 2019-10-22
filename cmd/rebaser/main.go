package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/local"
	"github.com/buildpack/imgutil/remote"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/auth"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/lifecycle/metadata"
)

var (
	imageNames   []string
	runImageRef  string
	useDaemon    bool
	useHelpers   bool
	printVersion bool
	logLevel     string
)

func init() {
	cmd.FlagRunImage(&runImageRef)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagVersion(&printVersion)
	cmd.FlagLogLevel(&logLevel)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()

	if printVersion {
		cmd.ExitWithVersion()
	}

	if err := cmd.SetLogLevel(logLevel); err != nil {
		cmd.Exit(err)
	}

	imageNames = flag.Args()

	if len(imageNames) == 0 {
		cmd.Exit(cmd.FailErrCode(errors.New("at least one image argument is required"), cmd.CodeInvalidArgs, "parse arguments"))
	}

	cmd.Exit(rebase())
}

func rebase() error {
	rebaser := &lifecycle.Rebaser{
		Logger: cmd.Logger,
	}

	registry, err := image.EnsureSingleRegistry(imageNames...)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "parse arguments")
	}

	var newImage func(string) (imgutil.Image, error)
	if useDaemon {
		newImage = func(ref string) (imgutil.Image, error) {
			dockerClient, err := cmd.DockerClient()
			if err != nil {
				return nil, err
			}
			return local.NewImage(ref, dockerClient, local.FromBaseImage(ref))
		}
	} else {
		newImage = func(ref string) (imgutil.Image, error) {
			if useHelpers {
				if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), ref); err != nil {
					return nil, cmd.FailErr(err, "setup credential helpers")
				}
			}
			return remote.NewImage(ref, auth.EnvKeychain(cmd.EnvRegistryAuth), remote.FromBaseImage(ref))
		}
	}

	appImage, err := newImage(imageNames[0])
	if err != nil || !appImage.Found() {
		return cmd.FailErr(err, "access image to rebase")
	}

	md, err := metadata.GetLayersMetadata(appImage)
	if err != nil {
		return err
	}

	if runImageRef == "" {
		if md.Stack.RunImage.Image == "" {
			return cmd.FailErrCode(errors.New("-image is required when there is no stack metadata available"), cmd.CodeInvalidArgs, "parse arguments")
		}
		runImageRef, err = md.Stack.BestRunImageMirror(registry)
		if err != nil {
			return err
		}
	}

	newBaseImage, err := newImage(runImageRef)
	if err != nil || !newBaseImage.Found() {
		return cmd.FailErr(err, "access run image")
	}

	if err := rebaser.Rebase(appImage, newBaseImage, imageNames[1:]); err != nil {
		if _, ok := err.(*imgutil.SaveError); ok {
			return cmd.FailErrCode(err, cmd.CodeFailedSave, "export")
		}
		return cmd.FailErr(err, "rebase")
	}

	return nil
}
