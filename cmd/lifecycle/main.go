package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/platform"
)

func main() {
	phase := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	switch phase {
	case "detector":
		cli.Run(&detectCmd{Platform: platform.NewPlatformFor(platformAPIWithExitOnError())}, phase, false)
	case "analyzer":
		cli.Run(&analyzeCmd{Platform: platform.NewPlatformFor(platformAPIWithExitOnError())}, phase, false)
	case "restorer":
		cli.Run(&restoreCmd{Platform: platform.NewPlatformFor(platformAPIWithExitOnError())}, phase, false)
	case "builder":
		cli.Run(&buildCmd{Platform: platform.NewPlatformFor(platformAPIWithExitOnError())}, phase, false)
	case "exporter":
		cli.Run(&exportCmd{Platform: platform.NewPlatformFor(platformAPIWithExitOnError())}, phase, false)
	case "creator":
		cli.Run(&createCmd{Platform: platform.NewPlatformFor(platformAPIWithExitOnError())}, phase, false)
	case "extender":
		cli.Run(&extendCmd{Platform: platform.NewPlatformFor(platformAPIWithExitOnError())}, phase, false)
	case "rebaser":
		cli.Run(&rebaseCmd{Platform: platform.NewPlatformFor(platformAPIWithExitOnError())}, phase, false)
	default:
		if len(os.Args) < 2 {
			cmd.Exit(cmd.FailCode(cmd.CodeForInvalidArgs, "parse arguments"))
		}
		if os.Args[1] == "-version" {
			cmd.ExitWithVersion()
		}
		subcommand(platformAPIWithExitOnError())
	}
}

func platformAPIWithExitOnError() string {
	platformAPI := cmd.EnvOrDefault(platform.EnvPlatformAPI, platform.DefaultPlatformAPI)
	if err := cmd.VerifyPlatformAPI(platformAPI, cmd.DefaultLogger); err != nil {
		cmd.Exit(err)
	}
	return platformAPI
}

func subcommand(platformAPI string) {
	phase := filepath.Base(os.Args[1])
	switch phase {
	case "detect":
		cli.Run(&detectCmd{Platform: platform.NewPlatformFor(platformAPI)}, phase, true)
	case "analyze":
		cli.Run(&analyzeCmd{Platform: platform.NewPlatformFor(platformAPI)}, phase, true)
	case "restore":
		cli.Run(&restoreCmd{Platform: platform.NewPlatformFor(platformAPI)}, phase, true)
	case "build":
		cli.Run(&buildCmd{Platform: platform.NewPlatformFor(platformAPI)}, phase, true)
	case "export":
		cli.Run(&exportCmd{Platform: platform.NewPlatformFor(platformAPI)}, phase, true)
	case "rebase":
		cli.Run(&rebaseCmd{Platform: platform.NewPlatformFor(platformAPI)}, phase, true)
	case "create":
		cli.Run(&createCmd{Platform: platform.NewPlatformFor(platformAPI)}, phase, true)
	case "extend":
		cli.Run(&extendCmd{Platform: platform.NewPlatformFor(platformAPI)}, phase, true)
	default:
		cmd.Exit(cmd.FailCode(cmd.CodeForInvalidArgs, "unknown phase:", phase))
	}
}

// handlers

type DefaultCacheHandler struct {
	keychain authn.Keychain
}

func NewCacheHandler(keychain authn.Keychain) *DefaultCacheHandler {
	return &DefaultCacheHandler{
		keychain: keychain,
	}
}

func (ch *DefaultCacheHandler) InitCache(cacheImageRef string, cacheDir string) (lifecycle.Cache, error) {
	var (
		cacheStore lifecycle.Cache
		err        error
	)
	if cacheImageRef != "" {
		cacheStore, err = cache.NewImageCacheFromName(cacheImageRef, ch.keychain, cmd.DefaultLogger)
		if err != nil {
			return nil, errors.Wrap(err, "creating image cache")
		}
	} else if cacheDir != "" {
		cacheStore, err = cache.NewVolumeCache(cacheDir)
		if err != nil {
			return nil, errors.Wrap(err, "creating volume cache")
		}
	}
	return cacheStore, nil
}

type DefaultRegistryHandler struct {
	keychain authn.Keychain
}

func NewRegistryHandler(keychain authn.Keychain) *DefaultRegistryHandler {
	return &DefaultRegistryHandler{
		keychain: keychain,
	}
}

func (rv *DefaultRegistryHandler) EnsureReadAccess(imageRefs ...string) error {
	for _, imageRef := range imageRefs {
		if err := verifyReadAccess(imageRef, rv.keychain); err != nil {
			return err
		}
	}
	return nil
}

func (rv *DefaultRegistryHandler) EnsureWriteAccess(imageRefs ...string) error {
	for _, imageRef := range imageRefs {
		if err := verifyReadWriteAccess(imageRef, rv.keychain); err != nil {
			return err
		}
	}
	return nil
}

func verifyReadAccess(imageRef string, keychain authn.Keychain) error {
	if imageRef == "" {
		return nil
	}
	img, _ := remote.NewImage(imageRef, keychain)
	if !img.CheckReadAccess() {
		return errors.Errorf("ensure registry read access to %s", imageRef)
	}
	return nil
}

func verifyReadWriteAccess(imageRef string, keychain authn.Keychain) error {
	if imageRef == "" {
		return nil
	}
	img, _ := remote.NewImage(imageRef, keychain)
	if !img.CheckReadWriteAccess() {
		return errors.Errorf("ensure registry read/write access to %s", imageRef)
	}
	return nil
}

// helpers

func initCache(cacheImageTag, cacheDir string, keychain authn.Keychain) (lifecycle.Cache, error) {
	var (
		cacheStore lifecycle.Cache
		err        error
	)
	if cacheImageTag != "" {
		cacheStore, err = cache.NewImageCacheFromName(cacheImageTag, keychain, cmd.DefaultLogger)
		if err != nil {
			return nil, cmd.FailErr(err, "create image cache")
		}
	} else if cacheDir != "" {
		cacheStore, err = cache.NewVolumeCache(cacheDir)
		if err != nil {
			return nil, cmd.FailErr(err, "create volume cache")
		}
	}
	return cacheStore, nil
}

func verifyBuildpackApis(group buildpack.Group) error {
	for _, bp := range group.Group {
		if bp.API == "" {
			// if this group was generated by this lifecycle bp.API should be set
			// but if for some reason it isn't default to 0.2
			bp.API = "0.2"
		}
		if err := cmd.VerifyBuildpackAPI(buildpack.KindBuildpack, bp.String(), bp.API, cmd.DefaultLogger); err != nil { // FIXME: when exporter is extensions-aware, this function call should be modified to provide the right module kind
			return err
		}
	}
	return nil
}
