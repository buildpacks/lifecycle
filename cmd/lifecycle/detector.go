package main

import (
	"errors"
	"os"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/priv"
)

type detectCmd struct {
	// flags: inputs
	detectArgs

	// flags: paths to write outputs
	groupPath string
	planPath  string
}

type detectArgs struct {
	// inputs needed when run by creator
	buildpacksDir string
	appDir        string
	platformDir   string
	orderPath     string
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

func (d *detectCmd) Privileges() error {
	// detector should never be run with privileges
	if priv.IsPrivileged() {
		return cmd.FailErr(errors.New("refusing to run as root"), "build")
	}
	return nil
}

func (d *detectCmd) Exec() error {
	group, plan, err := d.detect()
	if err != nil {
		return err
	}
	return d.writeData(group, plan)
}

func (da detectArgs) detect() (lifecycle.BuildpackGroup, lifecycle.BuildPlan, error) {
	order, err := lifecycle.ReadOrder(da.orderPath)
	if err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read buildpack order file")
	}

	envv := env.NewBuildEnv(os.Environ())
	fullEnv, err := envv.WithPlatform(da.platformDir)
	if err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read full env")
	}
	group, plan, err := order.Detect(&lifecycle.DetectConfig{
		FullEnv:       fullEnv,
		ClearEnv:      envv.List(),
		AppDir:        da.appDir,
		PlatformDir:   da.platformDir,
		BuildpacksDir: da.buildpacksDir,
		Logger:        cmd.DefaultLogger,
	})
	if err != nil {
		switch err {
		case lifecycle.ErrFailedDetection:
			cmd.DefaultLogger.Error("No buildpack groups passed detection.")
			cmd.DefaultLogger.Error("Please check that you are running against the correct path.")
			return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErrCode(err, cmd.CodeFailedDetect, "detect")
		case lifecycle.ErrBuildpack:
			cmd.DefaultLogger.Error("No buildpack groups passed detection.")
			return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErrCode(err, cmd.CodeFailedDetectWithErrors, "detect")
		default:
			return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErrCode(err, 1, "detect")
		}
	}

	return group, plan, nil
}

func (d *detectCmd) writeData(group lifecycle.BuildpackGroup, plan lifecycle.BuildPlan) error {
	if err := lifecycle.WriteTOML(d.groupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if err := lifecycle.WriteTOML(d.planPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}
	return nil
}
