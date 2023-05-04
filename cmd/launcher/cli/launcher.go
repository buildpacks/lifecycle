package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/heroku/color"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	launchenv "github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/internal/path"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform/config"
	"github.com/buildpacks/lifecycle/platform/exit"
	"github.com/buildpacks/lifecycle/platform/exit/fail"
	platform "github.com/buildpacks/lifecycle/platform/launch"
	"github.com/buildpacks/lifecycle/platform/launch/env"
)

const KindBuildpack = "buildpack"

func RunLaunch() error {
	color.Disable(env.NoColor)

	platformAPI := envOrDefault(env.PlatformAPI, platform.DefaultPlatformAPI)
	if err := config.VerifyPlatformAPI(platformAPI, cmd.DefaultLogger); err != nil {
		cmd.Exit(err)
	}
	p := platform.NewPlatform(platformAPI)

	var md launch.Metadata
	if _, err := toml.DecodeFile(launch.GetMetadataFilePath(envOrDefault(env.LayersDir, platform.DefaultLayersDir)), &md); err != nil {
		return exit.ErrorFromErr(err, "read metadata")
	}
	if err := verifyBuildpackAPIs(md.Buildpacks); err != nil {
		return err
	}

	defaultProcessType := defaultProcessType(p.API(), md)

	launcher := &launch.Launcher{
		DefaultProcessType: defaultProcessType,
		LayersDir:          envOrDefault(env.LayersDir, platform.DefaultLayersDir),
		AppDir:             envOrDefault(env.AppDir, platform.DefaultAppDir),
		PlatformAPI:        p.API(),
		Processes:          md.Processes,
		Buildpacks:         md.Buildpacks,
		Env:                launchenv.NewLaunchEnv(os.Environ(), launch.ProcessDir, launch.LifecycleDir),
		Exec:               launch.OSExecFunc,
		ExecD:              launch.NewExecDRunner(),
		Shell:              launch.DefaultShell,
		Setenv:             os.Setenv,
	}

	if err := launcher.Launch(os.Args[0], os.Args[1:]); err != nil {
		return exit.ErrorFromErrAndCode(err, p.CodeFor(fail.LaunchError), "launch")
	}
	return nil
}

func envOrDefault(envVal string, defaultVal string) string {
	if envVal != "" {
		return envVal
	}
	return defaultVal
}

func defaultProcessType(platformAPI *api.Version, launchMD launch.Metadata) string {
	if platformAPI.LessThan("0.4") {
		return envOrDefault(env.ProcessType, platform.DefaultProcessType)
	}
	if pType := os.Getenv(env.ProcessType); pType != "" {
		cmd.DefaultLogger.Warnf("%s is not supported in Platform API %s", env.ProcessType, platformAPI)
		cmd.DefaultLogger.Warnf("Run with ENTRYPOINT '%s' to invoke the '%s' process type", pType, pType)
	}

	_, process := filepath.Split(os.Args[0])
	processType := strings.TrimSuffix(process, path.ExecExt)
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
		if err := config.VerifyBuildpackAPI(KindBuildpack, bp.ID, bp.API, cmd.DefaultLogger); err != nil {
			return err
		}
	}
	return nil
}
