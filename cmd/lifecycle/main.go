package main

import (
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
)

func main() {
	switch filepath.Base(os.Args[0]) {
	case "detector":
		cmd.Run(&detectCmd{})
	case "analyzer":
		cmd.Run(&analyzeCmd{})
	case "restorer":
		cmd.Run(&restoreCmd{})
	case "builder":
		cmd.Run(&buildCmd{})
	case "exporter":
		cmd.Run(&exportCmd{})
	case "rebaser":
		cmd.Run(&rebaseCmd{})
	default:
		if len(os.Args) < 2 {
			cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments"))
		}
		if os.Args[1] == "-version" {
			cmd.ExitWithVersion()
		}
		subcommand()
	}
}

func subcommand() {
	phase := filepath.Base(os.Args[1])
	switch phase {
	case "detect":
		cmd.Run(&detectCmd{})
	case "analyze":
		cmd.Run(&analyzeCmd{})
	case "restore":
		cmd.Run(&restoreCmd{})
	case "build":
		cmd.Run(&buildCmd{})
	case "export":
		cmd.Run(&exportCmd{})
	case "rebase":
		cmd.Run(&rebaseCmd{})
	default:
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "unknown phase:", phase))
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
