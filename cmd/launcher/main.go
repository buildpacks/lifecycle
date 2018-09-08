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

func main() {
	cmd.Exit(launch())
}

func launch() error {
	defaultProcessType := "web"
	if v := os.Getenv("PACK_PROCESS_TYPE"); v != "" {
		defaultProcessType = v
	}

	var metadata lifecycle.BuildMetadata
	metadataPath := filepath.Join(lifecycle.DefaultLaunchDir, "config", "metadata.toml")
	if _, err := toml.DecodeFile(metadataPath, &metadata); err != nil {
		return cmd.FailErr(err, "read metadata")
	}

	launcher := &lifecycle.Launcher{
		DefaultProcessType: defaultProcessType,
		DefaultLaunchDir:   lifecycle.DefaultLaunchDir,
		Processes:          metadata.Processes,
		Buildpacks:         metadata.Buildpacks,
		Exec:               syscall.Exec,
	}

	if err := launcher.Launch(os.Args[0], strings.Join(os.Args[1:], " ")); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedLaunch, "launch")
	}
	return nil
}
