package main

import (
	"errors"
	"fmt"
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
	groupPath      string
	stackGroupPath string
	planPath       string
}

type detectArgs struct {
	// inputs needed when run by creator
	buildpacksDir      string
	appDir             string
	platformDir        string
	orderPath          string
	stackBuildpacksDir string
}

func (d *detectCmd) Init() {
	cmd.FlagBuildpacksDir(&d.buildpacksDir)
	cmd.FlagAppDir(&d.appDir)
	cmd.FlagPlatformDir(&d.platformDir)
	cmd.FlagStackBuildpacksDir(&d.stackBuildpacksDir)
	cmd.FlagOrderPath(&d.orderPath)
	cmd.FlagGroupPath(&d.groupPath)
	cmd.FlagStackGroupPath(&d.stackGroupPath)
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
	if err := da.verifyBuildpackApis(order); err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, err
	}

	envv := env.NewBuildEnv(os.Environ())
	fullEnv, err := envv.WithPlatform(da.platformDir)
	if err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read full env")
	}
	group, plan, err := order.Detect(&lifecycle.DetectConfig{
		FullEnv:            fullEnv,
		ClearEnv:           envv.List(),
		AppDir:             da.appDir,
		PlatformDir:        da.platformDir,
		BuildpacksDir:      da.buildpacksDir,
		Logger:             cmd.DefaultLogger,
		StackBuildpacksDir: da.stackBuildpacksDir,
	})
	if err != nil {
		switch err := err.(type) {
		case *lifecycle.Error:
			switch err.Type {
			case lifecycle.ErrTypeFailedDetection:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				cmd.DefaultLogger.Error("Please check that you are running against the correct path.")
				return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErrCode(err, cmd.CodeFailedDetect, "detect")
			case lifecycle.ErrTypeBuildpack:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErrCode(err, cmd.CodeFailedDetectWithErrors, "detect")
			default:
				return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErrCode(err, cmd.CodeDetectError, "detect")
			}
		default:
			return lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErrCode(err, cmd.CodeDetectError, "detect")
		}
	}

	return group, plan, nil
}

func (da detectArgs) verifyBuildpackApis(order lifecycle.BuildpackOrder) error {
	for _, group := range order {
		for _, bp := range group.Group {
			bpTOML, err := bp.Lookup(da.buildpacksDir)
			if err != nil {
				return cmd.FailErr(err, fmt.Sprintf("lookup buildpack.toml for buildpack '%s'", bp.String()))
			}
			if err := cmd.VerifyBuildpackAPI(bp.String(), bpTOML.API); err != nil {
				return err
			}
		}
	}
	return nil
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
