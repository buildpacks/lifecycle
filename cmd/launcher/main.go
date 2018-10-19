package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
)

var (
	launchDir string
)

func main() {
	cmd.Exit(launch())
}

func launch() error {
	defaultProcessType := "web"
	if v := os.Getenv("PACK_PROCESS_TYPE"); v != "" {
		defaultProcessType = v
	}

	launchDir := cmd.DefaultLaunchDir
	if v := os.Getenv(lifecycle.EnvLaunchDir); v != "" {
		launchDir = v
	}
	os.Unsetenv(lifecycle.EnvLaunchDir)

	var metadata lifecycle.BuildMetadata
	metadataPath := filepath.Join(launchDir, "config", "metadata.toml")
	if _, err := toml.DecodeFile(metadataPath, &metadata); err != nil {
		return cmd.FailErr(err, "read metadata")
	}

	launcher := &lifecycle.Launcher{
		DefaultProcessType: defaultProcessType,
		LaunchDir:          launchDir,
		Processes:          metadata.Processes,
		Buildpacks:         metadata.Buildpacks,
		Exec:               syscall.Exec,
	}

	if err := launcher.Launch(os.Args[0], strings.Join(os.Args[1:], " ")); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedLaunch, "launch")
	}
	return nil
}
