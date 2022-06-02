package main

import (
	"errors"
	"fmt"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/guard"
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
	layersDir     string
	platformDir   string
	orderPath     string

	platform Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (d *detectCmd) DefineFlags() {
	cli.FlagBuildpacksDir(&d.buildpacksDir)
	cli.FlagAppDir(&d.appDir)
	cli.FlagLayersDir(&d.layersDir)
	cli.FlagPlatformDir(&d.platformDir)
	cli.FlagOrderPath(&d.orderPath)
	cli.FlagGroupPath(&d.groupPath)
	cli.FlagPlanPath(&d.planPath)
}

// Args validates arguments and flags, and fills in default values.
func (d *detectCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), platform.CodeForInvalidArgs, "parse arguments")
	}

	if d.groupPath == platform.PlaceholderGroupPath {
		d.groupPath = cli.DefaultGroupPath(d.platform.API().String(), d.layersDir)
	}

	if d.planPath == platform.PlaceholderPlanPath {
		d.planPath = cli.DefaultPlanPath(d.platform.API().String(), d.layersDir)
	}

	if d.orderPath == platform.PlaceholderOrderPath {
		d.orderPath = cli.DefaultOrderPath(d.platform.API().String(), d.layersDir)
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
	order, err := buildpack.ReadOrder(da.orderPath)
	if err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErr(err, "read buildpack order file")
	}
	if err := da.verifyBuildpackApis(order); err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, err
	}

	detector, err := lifecycle.NewDetector(
		buildpack.DetectConfig{
			AppDir:      da.appDir,
			PlatformDir: da.platformDir,
			Logger:      cmd.DefaultLogger,
		},
		da.buildpacksDir,
		da.platform,
	)
	if err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErr(err, "initialize detector")
	}
	group, plan, err := detector.Detect(order)
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

func (da detectArgs) verifyBuildpackApis(order buildpack.Order) error {
	store, err := buildpack.NewBuildpackStore(da.buildpacksDir)
	if err != nil {
		return err
	}
	for _, group := range order {
		for _, groupBp := range group.Group {
			bp, err := store.Lookup(groupBp.ID, groupBp.Version)
			if err != nil {
				return cmd.FailErr(err, fmt.Sprintf("lookup buildpack.toml for buildpack '%s'", groupBp.String()))
			}
			if err := guard.BuildpackAPI(groupBp.String(), bp.ConfigFile().API, buildpack.APIs, cmd.DefaultLogger); err != nil {
				return cmd.FailErrCode(err, platform.CodeForIncompatibleBuildpackAPI, "set buildpack API")
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
