package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/heroku/color"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
)

func main() {
	cmd.Exit(runLaunch())
}

func runLaunch() error {
	color.Disable(cmd.BoolEnv(cmd.EnvNoColor))

	platformAPI := cmd.EnvOrDefault(cmd.EnvPlatformAPI, cmd.DefaultPlatformAPI)
	if err := cmd.VerifyPlatformAPI(platformAPI); err != nil {
		cmd.Exit(err)
	}

	var md launch.Metadata
	if _, err := toml.DecodeFile(launch.GetMetadataFilePath(cmd.EnvOrDefault(cmd.EnvLayersDir, cmd.DefaultLayersDir)), &md); err != nil {
		return cmd.FailErr(err, "read metadata")
	}
	if err := verifyBuildpackAPIs(md.Buildpacks); err != nil {
		return err
	}

	defaultProcessType := defaultProcessType(api.MustParse(platformAPI), md)

	launcher := &launch.Launcher{
		DefaultProcessType: defaultProcessType,
		LayersDir:          cmd.EnvOrDefault(cmd.EnvLayersDir, cmd.DefaultLayersDir),
		AppDir:             cmd.EnvOrDefault(cmd.EnvAppDir, cmd.DefaultAppDir),
		PlatformAPI:        api.MustParse(platformAPI),
		Processes:          md.Processes,
		Buildpacks:         md.Buildpacks,
		Env:                env.NewLaunchEnv(os.Environ(), launch.ProcessDir, launch.LifecycleDir),
		Exec:               launch.OSExecFunc,
		ExecD:              launch.NewExecDRunner(),
		Shell:              launch.DefaultShell,
		Setenv:             os.Setenv,
	}

	if err := launcher.Launch(os.Args[0], os.Args[1:]); err != nil {
		return cmd.FailErrCode(err, cmd.CodeLaunchError, "launch")
	}
	return nil
}

func defaultProcessType(platformAPI *api.Version, launchMD launch.Metadata) string {
	if platformAPI.Compare(api.MustParse("0.4")) < 0 {
		return cmd.EnvOrDefault(cmd.EnvProcessType, cmd.DefaultProcessType)
	}
	if pType := os.Getenv(cmd.EnvProcessType); pType != "" {
		cmd.DefaultLogger.Warnf("CNB_PROCESS_TYPE is not supported in Platform API %s", platformAPI)
		cmd.DefaultLogger.Warnf("Run with ENTRYPOINT '%s' to invoke the '%s' process type", pType, pType)
	}
	process := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	if _, ok := launchMD.FindProcessType(process); ok {
		return process
	}
	return ""
}

func verifyBuildpackAPIs(bps []launch.Buildpack) error {
	for _, bp := range bps {
		if bp.API == "" {
			// If the same lifecycle is used for build and launcher we should never end up here
			// but if for some reason we do, default to 0.2
			bp.API = "0.2"
		}
		if err := cmd.VerifyBuildpackAPI(bp.ID, bp.API); err != nil {
			return err
		}
	}
	return nil
}
