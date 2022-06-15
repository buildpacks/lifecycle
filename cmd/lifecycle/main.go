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
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
)

type Platform interface {
	API() *api.Version
	CodeFor(errType platform.LifecycleExitError) int
	ResolveAnalyze(inputs platform.AnalyzeInputs, logger log.Logger) (platform.AnalyzeInputs, error)
	ResolveDetect(inputs platform.DetectInputs) (platform.DetectInputs, error)
}

func main() {
	platformAPI := cmd.EnvOrDefault(cmd.EnvPlatformAPI, cmd.DefaultPlatformAPI)
	if err := cmd.VerifyPlatformAPI(platformAPI); err != nil {
		cmd.Exit(err)
	}

	p := platform.NewPlatform(platformAPI)

	switch strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0])) {
	case "detector":
		cmd.Run(&detectCmd{platform: p}, false)
	case "analyzer":
		cmd.Run(&analyzeCmd{platform: p}, false)
	case "restorer":
		cmd.Run(&restoreCmd{restoreArgs: restoreArgs{platform: p}}, false)
	case "builder":
		cmd.Run(&buildCmd{buildArgs: buildArgs{platform: p}}, false)
	case "exporter":
		cmd.Run(&exportCmd{exportArgs: exportArgs{platform: p}}, false)
	case "rebaser":
		cmd.Run(&rebaseCmd{platform: p}, false)
	case "creator":
		cmd.Run(&createCmd{platform: p}, false)
	default:
		if len(os.Args) < 2 {
			cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments"))
		}
		if os.Args[1] == "-version" {
			cmd.ExitWithVersion()
		}
		subcommand(p)
	}
}

func subcommand(p Platform) {
	phase := filepath.Base(os.Args[1])
	switch phase {
	case "detect":
		cmd.Run(&detectCmd{platform: p}, true)
	case "analyze":
		cmd.Run(&analyzeCmd{platform: p}, true)
	case "restore":
		cmd.Run(&restoreCmd{restoreArgs: restoreArgs{platform: p}}, true)
	case "build":
		cmd.Run(&buildCmd{buildArgs: buildArgs{platform: p}}, true)
	case "export":
		cmd.Run(&exportCmd{exportArgs: exportArgs{platform: p}}, true)
	case "rebase":
		cmd.Run(&rebaseCmd{platform: p}, true)
	case "create":
		cmd.Run(&createCmd{platform: p}, true)
	default:
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "unknown phase:", phase))
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

func appendNotEmpty(slice []string, elems ...string) []string {
	for _, v := range elems {
		if v != "" {
			slice = append(slice, v)
		}
	}
	return slice
}

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
		if err := cmd.VerifyBuildpackAPI(buildpack.KindBuildpack, bp.String(), bp.API, cmd.DefaultLogger); err != nil { // TODO: when builder and exporter are extensions-aware, this function call should be modified to provide the right module kind
			return err
		}
	}
	return nil
}
