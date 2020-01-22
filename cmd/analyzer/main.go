package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
)

var (
	analyzedPath  string
	cacheDir      string
	cacheImageTag string
	groupPath     string
	imageName     string
	layersDir     string
	skipLayers    bool
	useDaemon     bool
	useHelpers    bool
	uid           int
	gid           int
	printVersion  bool
	logLevel      string
)

func init() {
	cmd.FlagAnalyzedPath(&analyzedPath)
	cmd.FlagCacheDir(&cacheDir)
	cmd.FlagCacheImage(&cacheImageTag)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagSkipLayers(&skipLayers)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
	cmd.FlagVersion(&printVersion)
	cmd.FlagLogLevel(&logLevel)
}

func verifyCompatibility() error {
	platformsAPI := os.Getenv("CNB_PLATFORM_API")
	if platformsAPI != "" {
		providedVersion, err := api.NewVersion(platformsAPI)
		if err != nil {
			return err
		}

		lcPlatformAPI := api.MustParse(cmd.PlatformAPI)
		if !lcPlatformAPI.SupportsVersion(providedVersion) {
			return cmd.FailErrCode(
				fmt.Errorf("the Lifecycle's Platform API version is %s which is incompatible with Platform API version %s", lcPlatformAPI.String(), platformsAPI),
				11,
			)
		}
	}

	return nil
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	if err := verifyCompatibility(); err != nil {
		cmd.Exit(err)
	}

	flag.Parse()

	if printVersion {
		cmd.ExitWithVersion()
	}

	if err := cmd.SetLogLevel(logLevel); err != nil {
		cmd.Exit(err)
	}

	if flag.NArg() > 1 {
		cmd.Exit(cmd.FailErrCode(fmt.Errorf("received %d args expected 1", flag.NArg()), cmd.CodeInvalidArgs, "parse arguments"))
	}
	if flag.Arg(0) == "" {
		cmd.Exit(cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments"))
	}
	imageName = flag.Arg(0)

	if !skipLayers && cacheImageTag == "" && cacheDir == "" {
		cmd.Logger.Warn("Not restoring cached layer data, no cache flag specified.")
	}
	cmd.Exit(analyzer())
}

func analyzer() error {
	if useHelpers {
		if err := lifecycle.SetupCredHelpers(filepath.Join(os.Getenv("HOME"), ".docker"), imageName); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	group, err := lifecycle.ReadGroup(groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Group,
		LayersDir:  layersDir,
		Logger:     cmd.Logger,
		UID:        uid,
		GID:        gid,
		SkipLayers: skipLayers,
	}

	var img imgutil.Image
	if useDaemon {
		dockerClient, err := cmd.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "create docker client")
		}
		img, err = local.NewImage(
			imageName,
			dockerClient,
			local.FromBaseImage(imageName),
		)
		if err != nil {
			return cmd.FailErr(err, "access previous image")
		}
	} else {
		img, err = remote.NewImage(
			imageName,
			auth.EnvKeychain(cmd.EnvRegistryAuth),
			remote.FromBaseImage(imageName),
		)
		if err != nil {
			return cmd.FailErr(err, "access previous image")
		}
	}

	var cacheStore lifecycle.Cache
	if cacheImageTag != "" {
		cacheStore, err = cache.NewImageCacheFromName(cacheImageTag, auth.EnvKeychain(cmd.EnvRegistryAuth))
		if err != nil {
			return cmd.FailErr(err, "create image cache")
		}
	} else if cacheDir != "" {
		cacheStore, err = cache.NewVolumeCache(cacheDir)
		if err != nil {
			return cmd.FailErr(err, "create volume cache")
		}
	}

	md, err := analyzer.Analyze(img, cacheStore)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "analyze")
	}

	if err := lifecycle.WriteTOML(analyzedPath, md); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}
