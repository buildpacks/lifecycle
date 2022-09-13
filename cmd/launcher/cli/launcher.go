package cli

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
	platform "github.com/buildpacks/lifecycle/platform/launch"
)

func RunLaunch() error {
	color.Disable(cmd.BoolEnv(cmd.EnvNoColor))

	platformAPI := cmd.EnvOrDefault(cmd.EnvPlatformAPI, cmd.DefaultPlatformAPI)
	if err := cmd.VerifyPlatformAPI(platformAPI); err != nil {
		cmd.Exit(err)
	}
	p := platform.NewPlatform(platformAPI)

	var md launch.Metadata
	if _, err := toml.DecodeFile(launch.GetMetadataFilePath(cmd.EnvOrDefault(cmd.EnvLayersDir, cmd.DefaultLayersDir)), &md); err != nil {
		return cmd.FailErr(err, "read metadata")
	}
	if err := verifyBuildpackAPIs(md.Buildpacks); err != nil {
		return err
	}

	defaultProcessType := defaultProcessType(p.API(), md)

	launcher := &launch.Launcher{
		DefaultProcessType: defaultProcessType,
		LayersDir:          cmd.EnvOrDefault(cmd.EnvLayersDir, cmd.DefaultLayersDir),
		AppDir:             cmd.EnvOrDefault(cmd.EnvAppDir, cmd.DefaultAppDir),
		PlatformAPI:        p.API(),
		Processes:          md.Processes,
		Buildpacks:         md.Buildpacks,
		Env:                env.NewLaunchEnv(os.Environ(), launch.ProcessDir, launch.LifecycleDir),
		Exec:               launch.OSExecFunc,
		ExecD:              launch.NewExecDRunner(),
		Shell:              launch.DefaultShell,
		Setenv:             os.Setenv,
	}

	if err := launcher.Launch(os.Args[0], os.Args[1:]); err != nil {
		return cmd.FailErrCode(err, p.CodeFor(platform.LaunchError), "launch")
	}
	return nil
}

func defaultProcessType(platformAPI *api.Version, launchMD launch.Metadata) string {
	if platformAPI.LessThan("0.4") {
		return cmd.EnvOrDefault(cmd.EnvProcessType, cmd.DefaultProcessType)
	}
	if pType := os.Getenv(cmd.EnvProcessType); pType != "" {
		cmd.DefaultLogger.Warnf("CNB_PROCESS_TYPE is not supported in Platform API %s", platformAPI)
		cmd.DefaultLogger.Warnf("Run with ENTRYPOINT '%s' to invoke the '%s' process type", pType, pType)
	}

	_, process := filepath.Split(os.Args[0])
	processType := strings.TrimSuffix(process, platform.DefaultExecExt)
	if _, ok := launchMD.FindProcessType(processType); ok {
		return processType
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
