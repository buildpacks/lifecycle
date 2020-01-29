package main

import (
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
)

func commonFlags() {
	if printVersion {
		cmd.ExitWithVersion()
	}
	if err := cmd.SetLogLevel(logLevel); err != nil {
		cmd.Exit(err)
	}
}

func initCache(cacheImageTag, cacheDir string) (lifecycle.Cache, error) {
	var (
		cacheStore lifecycle.Cache
		err        error
	)
	if cacheImageTag != "" {
		cacheStore, err = cache.NewImageCacheFromName(cacheImageTag, auth.EnvKeychain(cmd.EnvRegistryAuth))
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

func initImage(imageName string, daemon bool) (imgutil.Image, error) {
	if daemon {
		dockerClient, err := cmd.DockerClient()
		if err != nil {
			return nil, cmd.FailErr(err, "create docker client")
		}
		return local.NewImage(
			imageName,
			dockerClient,
			local.FromBaseImage(imageName),
		)
	}
	return remote.NewImage(
		imageName,
		auth.EnvKeychain(cmd.EnvRegistryAuth),
		remote.FromBaseImage(imageName),
	)
}
