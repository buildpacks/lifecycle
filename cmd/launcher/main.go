package main

import (
	"os"
	"syscall"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/env"
)

func main() {
	cmd.Exit(launch())
}

func launch() error {
	defaultProcessType := cmd.DefaultProcessType
	if v := os.Getenv(cmd.EnvProcessType); v != "" {
		defaultProcessType = v
	}
	layersDir := cmd.DefaultLayersDir
	if v := os.Getenv(cmd.EnvLayersDir); v != "" {
		layersDir = v
	}
	appDir := cmd.DefaultAppDir
	if v := os.Getenv(cmd.EnvAppDir); v != "" {
		appDir = v
	}

	var md lifecycle.BuildMetadata
	if _, err := toml.DecodeFile(lifecycle.MetadataFilePath(layersDir), &md); err != nil {
		return cmd.FailErr(err, "read metadata")
	}

	launcher := &lifecycle.Launcher{
		DefaultProcessType: defaultProcessType,
		LayersDir:          layersDir,
		AppDir:             appDir,
		Processes:          md.Processes,
		Buildpacks:         md.Buildpacks,
		Env:                env.NewLaunchEnv(os.Environ()),
		Exec:               syscall.Exec,
	}

	if err := launcher.Launch(os.Args[0], os.Args[1:]); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedLaunch, "launch")
	}
	return nil
}
