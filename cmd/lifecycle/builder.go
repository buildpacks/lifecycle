package main

import (
	"errors"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type buildCmd struct {
	*platform.Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (b *buildCmd) DefineFlags() {
	switch {
	case b.PlatformAPI.AtLeast("0.11"):
		cli.FlagAppDir(&b.AppDir)
		cli.FlagBuildConfigDir(&b.BuildConfigDir)
		cli.FlagBuildpacksDir(&b.BuildpacksDir)
		cli.FlagGroupPath(&b.GroupPath)
		cli.FlagPlanPath(&b.PlanPath)
		cli.FlagPlatformDir(&b.PlatformDir)
	default:
		cli.FlagAppDir(&b.AppDir)
		cli.FlagBuildpacksDir(&b.BuildpacksDir)
		cli.FlagGroupPath(&b.GroupPath)
		cli.FlagPlanPath(&b.PlanPath)
		cli.FlagPlatformDir(&b.PlatformDir)
	}
}

// Args validates arguments and flags, and fills in default values.
func (b *buildCmd) Args(nargs int, _ []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	if err := platform.ResolveInputs(platform.Build, b.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	return nil
}

func (b *buildCmd) Privileges() error {
	// builder should never be run with privileges
	if priv.IsPrivileged() {
		return cmd.FailErr(errors.New("refusing to run as root"), "build")
	}
	return nil
}

func (b *buildCmd) Exec() error {
	group, plan, err := b.readData()
	if err != nil {
		return err
	}
	if err = verifyBuildpackApis(group); err != nil {
		return err
	}
	return b.build(group, plan)
}

func (b *buildCmd) build(group buildpack.Group, plan platform.BuildPlan) error {
	builder := &lifecycle.Builder{
		AppDir:         b.AppDir,
		BuildConfigDir: b.BuildConfigDir,
		LayersDir:      b.LayersDir,
		PlatformDir:    b.PlatformDir,
		BuildExecutor:  &buildpack.DefaultBuildExecutor{},
		DirStore:       platform.NewDirStore(b.BuildpacksDir, ""),
		Group:          group,
		Logger:         cmd.DefaultLogger,
		Out:            cmd.Stdout,
		Err:            cmd.Stderr,
		Plan:           plan,
		PlatformAPI:    b.PlatformAPI,
	}
	md, err := builder.Build()
	if err != nil {
		return b.unwrapBuildFail(err)
	}
	if err = encoding.WriteTOML(launch.GetMetadataFilePath(b.LayersDir), md); err != nil {
		return cmd.FailErr(err, "write build metadata")
	}
	return nil
}

func (b *buildCmd) unwrapBuildFail(err error) error {
	if err, ok := err.(*buildpack.Error); ok {
		if err.Type == buildpack.ErrTypeBuildpack {
			return cmd.FailErrCode(err.Cause(), b.CodeFor(platform.FailedBuildWithErrors), "build")
		}
	}
	return cmd.FailErrCode(err, b.CodeFor(platform.BuildError), "build")
}

func (b *buildCmd) readData() (buildpack.Group, platform.BuildPlan, error) {
	group, err := lifecycle.ReadGroup(b.GroupPath)
	if err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErr(err, "read buildpack group")
	}

	var plan platform.BuildPlan
	if _, err := toml.DecodeFile(b.PlanPath, &plan); err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErr(err, "parse detect plan")
	}
	return group, plan, nil
}
