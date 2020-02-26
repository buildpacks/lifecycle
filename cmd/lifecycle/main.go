package main

import (
	"context"
	"net"
	"os"
	"path/filepath"

	"github.com/docker/docker/client"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
)

func main() {
	if err := cmd.VerifyCompatibility(); err != nil {
		cmd.Exit(err)
	}

	switch filepath.Base(os.Args[0]) {
	case "detector":
		cmd.Run(&detectCmd{}, false)
	case "analyzer":
		cmd.Run(&analyzeCmd{}, false)
	case "restorer":
		cmd.Run(&restoreCmd{}, false)
	case "builder":
		cmd.Run(&buildCmd{}, false)
	case "exporter":
		cmd.Run(&exportCmd{}, false)
	case "rebaser":
		cmd.Run(&rebaseCmd{}, false)
	case "creator":
		cmd.Run(&createCmd{}, false)
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
		cmd.Run(&detectCmd{}, true)
	case "analyze":
		cmd.Run(&analyzeCmd{}, true)
	case "restore":
		cmd.Run(&restoreCmd{}, true)
	case "build":
		cmd.Run(&buildCmd{}, true)
	case "export":
		cmd.Run(&exportCmd{}, true)
	case "rebase":
		cmd.Run(&rebaseCmd{}, true)
	case "create":
		cmd.Run(&createCmd{}, true)
	default:
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "unknown phase:", phase))
	}
}

func dockerClient() (*client.Client, error) {
	var connOpt client.Opt
	if os.Getenv("DOCKER_HOST") != "" {
		connOpt = client.FromEnv
	} else {
		socketConn, err := net.Dial("unix", "/var/run/docker.sock")
		if err != nil {
			return nil, err
		}
		connOpt = client.WithDialContext(
			func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
				return socketConn, nil
			})
	}
	docker, err := client.NewClientWithOpts(connOpt, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.Wrap(err, "new docker client")
	}
	return docker, nil
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
