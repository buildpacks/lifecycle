package main

import (
	"errors"
	"fmt"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/inputs"
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
	extensionsDir string

	// inputs needed when run by creator
	buildpacksDir string
	appDir        string
	layersDir     string
	platformDir   string
	orderPath     string

	platform Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (d *detectCmd) DefineFlags() {
	switch {
	case d.platform.API().AtLeast("0.10"):
		cmd.FlagAppDir(&d.appDir)
		cmd.FlagBuildpacksDir(&d.buildpacksDir)
		cmd.FlagExtensionsDir(&d.extensionsDir)
		cmd.FlagGroupPath(&d.groupPath)
		cmd.FlagLayersDir(&d.layersDir)
		cmd.FlagOrderPath(&d.orderPath)
		cmd.FlagPlanPath(&d.planPath)
		cmd.FlagPlatformDir(&d.platformDir)
	default:
		cmd.FlagAppDir(&d.appDir)
		cmd.FlagBuildpacksDir(&d.buildpacksDir)
		cmd.FlagGroupPath(&d.groupPath)
		cmd.FlagLayersDir(&d.layersDir)
		cmd.FlagOrderPath(&d.orderPath)
		cmd.FlagPlanPath(&d.planPath)
		cmd.FlagPlatformDir(&d.platformDir)
	}
}

// Args validates arguments and flags, and fills in default values.
func (d *detectCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeInvalidArgs, "parse arguments")
	}

	if d.groupPath == cmd.PlaceholderGroupPath {
		d.groupPath = cmd.DefaultGroupPath(d.platform.API().String(), d.layersDir)
	}

	if d.planPath == cmd.PlaceholderPlanPath {
		d.planPath = cmd.DefaultPlanPath(d.platform.API().String(), d.layersDir)
	}

	if d.orderPath == cmd.PlaceholderOrderPath {
		d.orderPath = cmd.DefaultOrderPath(d.platform.API().String(), d.layersDir)
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

func (da detectArgs) detect() (buildpack.Group, platform.BuildPlan, error) {
	// read provided order path
	order, orderExt, err := buildpack.ReadOrder(da.orderPath)
	if err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErr(err, "read order file")
	}
	// initialize store from provided dir paths
	execStore, err := inputs.NewExecStore(da.buildpacksDir, da.extensionsDir)
	if err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErr(err, "initialize executable store")
	}
	// verify apis
	if err := da.verifyApis(execStore, order, orderExt); err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, err
	}
	// new detector
	detector := lifecycle.NewDetector(
		buildpack.DetectConfig{
			AppDir:      da.appDir,
			PlatformDir: da.platformDir,
			Logger:      cmd.DefaultLogger,
		},
		execStore,
	)
	// do detect
	group, plan, err := detector.Detect(order, nil)
	if err != nil {
		switch err := err.(type) {
		case *buildpack.Error:
			switch err.Type {
			case buildpack.ErrTypeFailedDetection:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				cmd.DefaultLogger.Error("Please check that you are running against the correct path.")
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, da.platform.CodeFor(platform.FailedDetect), "detect")
			case buildpack.ErrTypeBuildpack:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, da.platform.CodeFor(platform.FailedDetectWithErrors), "detect")
			default:
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, da.platform.CodeFor(platform.DetectError), "detect")
			}
		default:
			return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, da.platform.CodeFor(platform.DetectError), "detect")
		}
	}

	return group, plan, nil
}

func (da detectArgs) verifyApis(execStore *inputs.ExecStore, order buildpack.Order, orderExt buildpack.Order) error {
	for _, group := range order {
		for _, groupEl := range group.Group {
			foundBp, err := execStore.LookupBp(groupEl.ID, groupEl.Version)
			if err != nil {
				return cmd.FailErr(err, fmt.Sprintf("lookup buildpack.toml for buildpack '%s'", groupEl.String()))
			}
			if err := cmd.VerifyBuildpackAPI(groupEl.String(), foundBp.ConfigFile().API); err != nil {
				return err
			}
		}
	}
	for _, group := range orderExt {
		for _, groupEl := range group.Group {
			foundExt, err := execStore.LookupExt(groupEl.ID, groupEl.Version)
			if err != nil {
				return cmd.FailErr(err, fmt.Sprintf("lookup extension.toml for extension '%s'", groupEl.String()))
			}
			if err := cmd.VerifyBuildpackAPI(groupEl.String(), foundExt.ConfigFile().API); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *detectCmd) writeData(group buildpack.Group, plan platform.BuildPlan) error {
	if err := encoding.WriteTOML(d.groupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if err := encoding.WriteTOML(d.planPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}
	return nil
}
