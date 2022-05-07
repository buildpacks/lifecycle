package main

import (
	"errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type detectCmd struct {
	platform Platform
	platform.DetectInputs
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (d *detectCmd) DefineFlags() {
	switch {
	case d.platform.API().AtLeast("0.10"):
		cmd.FlagAppDir(&d.AppDir)
		cmd.FlagBuildpacksDir(&d.BuildpacksDir)
		cmd.FlagExtensionsDir(&d.ExtensionsDir)
		cmd.FlagGroupPath(&d.GroupPath)
		cmd.FlagLayersDir(&d.LayersDir)
		cmd.FlagOrderPath(&d.OrderPath)
		cmd.FlagPlanPath(&d.PlanPath)
		cmd.FlagPlatformDir(&d.PlatformDir)
	default:
		cmd.FlagAppDir(&d.AppDir)
		cmd.FlagBuildpacksDir(&d.BuildpacksDir)
		cmd.FlagGroupPath(&d.GroupPath)
		cmd.FlagLayersDir(&d.LayersDir)
		cmd.FlagOrderPath(&d.OrderPath)
		cmd.FlagPlanPath(&d.PlanPath)
		cmd.FlagPlatformDir(&d.PlatformDir)
	}
}

// Args validates arguments and flags, and fills in default values.
func (d *detectCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeInvalidArgs, "parse arguments")
	}

	var err error
	d.DetectInputs, err = d.platform.ResolveDetect(d.DetectInputs)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "resolve inputs")
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
	factory := lifecycle.NewDetectorFactory(d.platform.API(), lifecycle.NewConfigHandler(&cmd.APIVerifier{}))
	detector, err := factory.NewDetector(
		d.AppDir,
		d.BuildpacksDir,
		d.ExtensionsDir,
		d.OrderPath,
		d.PlatformDir,
		cmd.DefaultLogger,
	)
	if err != nil {
		return cmd.FailErr(err, "initialize detector")
	}
	group, plan, err := doDetect(detector, d.platform)
	if err != nil {
		return err // pass through error from doDetect
	}
	return d.writeData(group, plan)
}

func doDetect(detector *lifecycle.Detector, p Platform) (buildpack.Group, platform.BuildPlan, error) {
	group, plan, err := detector.Detect()
	if err != nil {
		switch err := err.(type) {
		case *buildpack.Error:
			switch err.Type {
			case buildpack.ErrTypeFailedDetection:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				cmd.DefaultLogger.Error("Please check that you are running against the correct path.")
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, p.CodeFor(platform.FailedDetect), "detect")
			case buildpack.ErrTypeBuildpack:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, p.CodeFor(platform.FailedDetectWithErrors), "detect")
			default:
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, p.CodeFor(platform.DetectError), "detect")
			}
		default:
			return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, p.CodeFor(platform.DetectError), "detect")
		}
	}
	return group, plan, nil
}

func (d *detectCmd) writeData(group buildpack.Group, plan platform.BuildPlan) error {
	if err := encoding.WriteTOML(d.GroupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}
	if err := encoding.WriteTOML(d.PlanPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}
	return nil
}
