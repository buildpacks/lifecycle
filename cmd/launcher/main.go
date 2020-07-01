package main

import (
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
)

func main() {
	cmd.Exit(runLaunch())
}

func runLaunch() error {
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

	var md launch.Metadata
	if _, err := toml.DecodeFile(launch.GetMetadataFilePath(layersDir), &md); err != nil {
		return cmd.FailErr(err, "read metadata")
	}

	launcher := &launch.Launcher{
		DefaultProcessType: defaultProcessType,
		LayersDir:          layersDir,
		AppDir:             appDir,
		Processes:          md.Processes,
		Buildpacks:         md.Buildpacks,
		Env:                env.NewLaunchEnv(os.Environ()),
		Exec:               launch.OSExecFunc,
		Setenv:             os.Setenv,
	}

	if err := launcher.Launch(os.Args[0], os.Args[1:]); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedLaunch, "launch")
	}
	return nil
}
