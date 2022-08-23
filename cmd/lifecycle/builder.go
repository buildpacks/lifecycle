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
	// flags: inputs
	groupPath string
	planPath  string
	buildArgs
}

type buildArgs struct {
	// inputs needed when run by creator
	buildpacksDir string
	layersDir     string
	appDir        string
	platformDir   string

	platform Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (b *buildCmd) DefineFlags() {
	cli.FlagBuildpacksDir(&b.buildpacksDir)
	cli.FlagGroupPath(&b.groupPath)
	cli.FlagPlanPath(&b.planPath)
	cli.FlagLayersDir(&b.layersDir)
	cli.FlagAppDir(&b.appDir)
	cli.FlagPlatformDir(&b.platformDir)
}

// Args validates arguments and flags, and fills in default values.
func (b *buildCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeForInvalidArgs, "parse arguments")
	}

	if b.groupPath == platform.PlaceholderGroupPath {
		b.groupPath = cli.DefaultGroupPath(b.platform.API().String(), b.layersDir)
	}

	if b.planPath == platform.PlaceholderPlanPath {
		b.planPath = cli.DefaultPlanPath(b.platform.API().String(), b.layersDir)
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
	if err := verifyBuildpackApis(group); err != nil {
		return err
	}
	return b.build(group, plan)
}

func (ba buildArgs) build(group buildpack.Group, plan platform.BuildPlan) error {
	dirStore, err := platform.NewDirStore(ba.buildpacksDir, "")
	if err != nil {
		return cmd.FailErrCode(err, ba.platform.CodeFor(platform.BuildError), "build")
	}

	builder := &lifecycle.Builder{
		AppDir:      ba.appDir,
		LayersDir:   ba.layersDir,
		PlatformDir: ba.platformDir,
		Platform:    ba.platform,
		Group:       group,
		Plan:        plan,
		Out:         cmd.Stdout,
		Err:         cmd.Stderr,
		Logger:      cmd.DefaultLogger,
		DirStore:    dirStore,
	}
	md, err := builder.Build()
	if err != nil {
		return ba.unwrapBuildFail(err)
	}

	if err := encoding.WriteTOML(launch.GetMetadataFilePath(ba.layersDir), md); err != nil {
		return cmd.FailErr(err, "write build metadata")
	}
	return nil
}

func (ba *buildArgs) unwrapBuildFail(err error) error {
	if err, ok := err.(*buildpack.Error); ok {
		if err.Type == buildpack.ErrTypeBuildpack {
			return cmd.FailErrCode(err.Cause(), ba.platform.CodeFor(platform.FailedBuildWithErrors), "build")
		}
	}
	return cmd.FailErrCode(err, ba.platform.CodeFor(platform.BuildError), "build")
}

func (b *buildCmd) readData() (buildpack.Group, platform.BuildPlan, error) {
	group, err := lifecycle.ReadGroup(b.groupPath)
	if err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErr(err, "read buildpack group")
	}

	var plan platform.BuildPlan
	if _, err := toml.DecodeFile(b.planPath, &plan); err != nil {
		return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErr(err, "parse detect plan")
	}
	return group, plan, nil
}
