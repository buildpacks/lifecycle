package main

import (
	"flag"
	"os"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

type detectFlags struct {
	buildpacksDir string
	appDir        string
	platformDir   string
	orderPath     string
	groupPath     string
	planPath      string
}

func parseDetectFlags() detectFlags {
	f := detectFlags{}
	cmd.FlagBuildpacksDir(&f.buildpacksDir)
	cmd.FlagAppDir(&f.appDir)
	cmd.FlagPlatformDir(&f.platformDir)
	cmd.FlagOrderPath(&f.orderPath)
	cmd.FlagGroupPath(&f.groupPath)
	cmd.FlagPlanPath(&f.planPath)
	flag.Parse()
	commonFlags()
	return f
}

func detect(f detectFlags) error {
	order, err := lifecycle.ReadOrder(f.orderPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack order file")
	}

	env := &lifecycle.Env{
		LookupEnv: os.LookupEnv,
		Getenv:    os.Getenv,
		Setenv:    os.Setenv,
		Unsetenv:  os.Unsetenv,
		Environ:   os.Environ,
		Map:       lifecycle.POSIXBuildEnv,
	}
	fullEnv, err := env.WithPlatform(f.platformDir)
	if err != nil {
		return cmd.FailErr(err, "read full env")
	}
	group, plan, err := order.Detect(&lifecycle.DetectConfig{
		FullEnv:       fullEnv,
		ClearEnv:      env.List(),
		AppDir:        f.appDir,
		PlatformDir:   f.platformDir,
		BuildpacksDir: f.buildpacksDir,
		Logger:        cmd.Logger,
	})
	if err != nil {
		if err == lifecycle.ErrFail {
			cmd.Logger.Error("No buildpack groups passed detection.")
			cmd.Logger.Error("Please check that you are running against the correct path.")
		}
		return cmd.FailErrCode(err, cmd.CodeFailedDetect, "detect")
	}

	if err := lifecycle.WriteTOML(f.groupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if err := lifecycle.WriteTOML(f.planPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}

	return nil
}
