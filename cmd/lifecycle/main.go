package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
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
	switch strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0])) {
	case "detector":
		cli.Run(&detectCmd{Platform: platform.NewPlatformFor(platform.Detect, platformAPIWithExitOnError())}, false)
	case "analyzer":
		cli.Run(&analyzeCmd{Platform: platform.NewPlatformFor(platform.Analyze, platformAPIWithExitOnError())}, false)
	case "restorer":
		cli.Run(&restoreCmd{Platform: platform.NewPlatformFor(platform.Restore, platformAPIWithExitOnError())}, false)
	case "builder":
		cli.Run(&buildCmd{Platform: platform.NewPlatformFor(platform.Build, platformAPIWithExitOnError())}, false)
	case "exporter":
		cli.Run(&exportCmd{Platform: platform.NewPlatformFor(platform.Export, platformAPIWithExitOnError())}, false)
	case "rebaser":
		cli.Run(&rebaseCmd{Platform: platform.NewPlatformFor(platform.Rebase, platformAPIWithExitOnError())}, false)
	case "creator":
		cli.Run(&createCmd{Platform: platform.NewPlatformFor(platform.Create, platformAPIWithExitOnError())}, false)
	case "extender":
		cli.Run(&extendCmd{Platform: platform.NewPlatformFor(platform.Extend, platformAPIWithExitOnError())}, false)
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
		cli.Run(&detectCmd{Platform: platform.NewPlatformFor(platform.Detect, platformAPI)}, true)
	case "analyze":
		cli.Run(&analyzeCmd{Platform: platform.NewPlatformFor(platform.Analyze, platformAPI)}, true)
	case "restore":
		cli.Run(&restoreCmd{Platform: platform.NewPlatformFor(platform.Restore, platformAPI)}, true)
	case "build":
		cli.Run(&buildCmd{Platform: platform.NewPlatformFor(platform.Build, platformAPI)}, true)
	case "export":
		cli.Run(&exportCmd{Platform: platform.NewPlatformFor(platform.Export, platformAPI)}, true)
	case "rebase":
		cli.Run(&rebaseCmd{Platform: platform.NewPlatformFor(platform.Rebase, platformAPI)}, true)
	case "create":
		cli.Run(&createCmd{Platform: platform.NewPlatformFor(platform.Create, platformAPI)}, true)
	case "extend":
		cli.Run(&extendCmd{Platform: platform.NewPlatformFor(platform.Extend, platformAPI)}, true)
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
		cacheStore, err = cache.NewImageCacheFromName(cacheImageRef, ch.keychain)
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

type DefaultImageHandler struct {
	docker   client.CommonAPIClient
	keychain authn.Keychain
}

func NewImageHandler(docker client.CommonAPIClient, keychain authn.Keychain) *DefaultImageHandler {
	return &DefaultImageHandler{
		docker:   docker,
		keychain: keychain,
	}
}

func (h *DefaultImageHandler) InitImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	if h.docker != nil {
		return local.NewImage(
			imageRef,
			h.docker,
			local.FromBaseImage(imageRef),
		)
	}

	return remote.NewImage(
		imageRef,
		h.keychain,
		remote.FromBaseImage(imageRef),
	)
}

func (h *DefaultImageHandler) Docker() bool {
	return h.docker != nil
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
		cacheStore, err = cache.NewImageCacheFromName(cacheImageTag, keychain)
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
