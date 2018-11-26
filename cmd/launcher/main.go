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
	layersDir string
	appDir    string
)

func main() {
	cmd.Exit(launch())
}

func launch() error {
	defaultProcessType := "web"
	if v := os.Getenv("PACK_PROCESS_TYPE"); v != "" {
		defaultProcessType = v
	}

	layersDir := cmd.DefaultLayersDir
	if v := os.Getenv(cmd.EnvLayersDir); v != "" {
		layersDir = v
	}
	os.Unsetenv(cmd.EnvLayersDir)

	appDir := cmd.DefaultAppDir
	if v := os.Getenv(cmd.EnvAppDir); v != "" {
		appDir = v
	}
	os.Unsetenv(cmd.EnvAppDir)

	var metadata lifecycle.BuildMetadata
	metadataPath := filepath.Join(layersDir, "config", "metadata.toml")
	if _, err := toml.DecodeFile(metadataPath, &metadata); err != nil {
		return cmd.FailErr(err, "read metadata")
	}

	env := &lifecycle.Env{
		Getenv:  os.Getenv,
		Setenv:  os.Setenv,
		Environ: os.Environ,
		Map:     lifecycle.POSIXLaunchEnv,
	}
	launcher := &lifecycle.Launcher{
		DefaultProcessType: defaultProcessType,
		LayersDir:          layersDir,
		AppDir:             appDir,
		Processes:          metadata.Processes,
		Buildpacks:         metadata.Buildpacks,
		Env:                env,
		Exec:               syscall.Exec,
	}

	if err := launcher.Launch(os.Args[0], strings.Join(os.Args[1:], " ")); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedLaunch, "launch")
	}
	return nil
}
