package main

import (
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"syscall"

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

// dockerClient constructs a client that can continue to talk to a root owned docker socket
// * even after the process drops privileges
func dockerClient() (*client.Client, error) {
	host := client.DefaultDockerHost
	if envHost := os.Getenv("DOCKER_HOST"); envHost != "" {
		host = envHost
	}
	hostURL, err := client.ParseHostURL(host)
	if err != nil {
		return nil, err
	}
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if shouldConnectSock(hostURL) {
		opt, err := connectSockOpt(hostURL)
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to docker socket")
		}
		opts = append(opts, opt)
	}
	return client.NewClientWithOpts(opts...)
}

type unclosableConn struct {
	net.Conn
}

func (*unclosableConn) Close() error {
	// don't close a connection we won't have privileges to reopen
	return nil
}

// connectSockOpt overrides the client dialer to always return the same socket connection
func connectSockOpt(host *url.URL) (client.Opt, error) {
	socketConn, err := net.Dial("unix", host.Path)
	if err != nil {
		return nil, err
	}
	return client.WithDialContext(func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
		return &unclosableConn{socketConn}, nil
	}), nil
}

// shouldConnectSock returns true if the docker host is a root owned unix domain socket
func shouldConnectSock(host *url.URL) bool {
	if host.Scheme != "unix" {
		return false
	}
	fi, err := os.Stat(host.Path)
	if err != nil {
		return false
	}
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok && stat.Uid == 0 {
		return true
	}
	return false
}
