package main

import (
	"flag"

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

func detector(f detectFlags) error {
	group, plan, err := detect(f.orderPath, f.platformDir, f.appDir, f.buildpacksDir)
	if err != nil {
		return err
	}

	if err := lifecycle.WriteTOML(f.groupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if err := lifecycle.WriteTOML(f.planPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}

	return nil
}

func detect(orderPath, platformDir, appDir, buildpacksDir string) (lifecycle.BuildpackGroup, lifecycle.BuildPlan, error) {
	order, err := lifecycle.ReadOrder(orderPath)
	if err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read buildpack order file")
	}

	fullEnv, err := env.WithPlatform(platformDir)
	if err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read full env")
	}
	group, plan, err := order.Detect(&lifecycle.DetectConfig{
		FullEnv:       fullEnv,
		ClearEnv:      env.List(),
		AppDir:        appDir,
		PlatformDir:   platformDir,
		BuildpacksDir: buildpacksDir,
		Logger:        cmd.Logger,
	})
	if err != nil {
		if err == lifecycle.ErrFail {
			cmd.Logger.Error("No buildpack groups passed detection.")
			cmd.Logger.Error("Please check that you are running against the correct path.")
		}
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErrCode(err, cmd.CodeFailedDetect, "detect")
	}
	return group, plan, nil
}
