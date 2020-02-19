package main

import (
	"errors"
	"os"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/env"
)

type detectCmd struct {
	buildpacksDir string
	appDir        string
	platformDir   string
	orderPath     string
	groupPath     string
	planPath      string
}

func (d *detectCmd) Init() {
	cmd.FlagBuildpacksDir(&d.buildpacksDir)
	cmd.FlagAppDir(&d.appDir)
	cmd.FlagPlatformDir(&d.platformDir)
	cmd.FlagOrderPath(&d.orderPath)
	cmd.FlagGroupPath(&d.groupPath)
	cmd.FlagPlanPath(&d.planPath)
}

func (d *detectCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeInvalidArgs, "parse arguments")
	}
	return nil
}

func (d *detectCmd) Exec() error {
	group, plan, err := detect(
		d.orderPath,
		d.platformDir,
		d.appDir,
		d.buildpacksDir,
		os.Getuid(),
		os.Getgid(),
	)
	if err != nil {
		return err
	}

	if err := lifecycle.WriteTOML(d.groupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if err := lifecycle.WriteTOML(d.planPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}

	return nil
}

func detect(orderPath, platformDir, appDir, buildpacksDir string, uid, gid int) (lifecycle.BuildpackGroup, lifecycle.BuildPlan, error) {
	order, err := lifecycle.ReadOrder(orderPath)
	if err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read buildpack order file")
	}

	envv := env.NewBuildEnv(os.Environ())
	fullEnv, err := envv.WithPlatform(platformDir)
	if err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read full env")
	}
	group, plan, err := order.Detect(&lifecycle.DetectConfig{
		FullEnv:       fullEnv,
		ClearEnv:      envv.List(),
		AppDir:        appDir,
		PlatformDir:   platformDir,
		BuildpacksDir: buildpacksDir,
		Logger:        cmd.Logger,
		UID:           uid,
		GID:           gid,
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
