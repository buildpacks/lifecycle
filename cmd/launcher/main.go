package main

import (
	"os"
	"syscall"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/metadata"
)

func main() {
	cmd.Exit(launch())
}

func launch() error {
	defaultProcessType := cmd.DefaultProcessType
	if v := os.Getenv(cmd.EnvProcessType); v != "" {
		defaultProcessType = v
	} else if v := os.Getenv(cmd.EnvProcessTypeLegacy); v != "" {
		defaultProcessType = v
	}
	_ = os.Unsetenv(cmd.EnvProcessType)
	_ = os.Unsetenv(cmd.EnvProcessTypeLegacy)

	layersDir := cmd.DefaultLayersDir
	if v := os.Getenv(cmd.EnvLayersDir); v != "" {
		layersDir = v
	}
	_ = os.Unsetenv(cmd.EnvLayersDir)

	appDir := cmd.DefaultAppDir
	if v := os.Getenv(cmd.EnvAppDir); v != "" {
		appDir = v
	}
	_ = os.Unsetenv(cmd.EnvAppDir)

	var md lifecycle.BuildMetadata
	metadataPath := metadata.FilePath(layersDir)
	if _, err := toml.DecodeFile(metadataPath, &md); err != nil {
		return cmd.FailErr(err, "read metadata")
	}

	env := &lifecycle.Env{
		LookupEnv: os.LookupEnv,
		Getenv:    os.Getenv,
		Setenv:    os.Setenv,
		Unsetenv:  os.Unsetenv,
		Environ:   os.Environ,
		Map:       lifecycle.POSIXLaunchEnv,
	}
	launcher := &lifecycle.Launcher{
		DefaultProcessType: defaultProcessType,
		LayersDir:          layersDir,
		AppDir:             appDir,
		Processes:          md.Processes,
		Buildpacks:         md.Buildpacks,
		Env:                env,
		Exec:               syscall.Exec,
	}

	if err := launcher.Launch(os.Args[0], os.Args[1:]); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedLaunch, "launch")
	}
	return nil
}
