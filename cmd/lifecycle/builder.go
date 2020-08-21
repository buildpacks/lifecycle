package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/priv"
	"github.com/buildpacks/lifecycle/snapshot"
)

type buildCmd struct {
	// flags: inputs
	groupPath      string
	stackGroupPath string
	planPath       string
	buildArgs
}

type buildArgs struct {
	// inputs needed when run by creator
	uid, gid      int
	buildpacksDir string
	layersDir     string
	appDir        string
	platformDir   string
	platformAPI   string
}

func (b *buildCmd) Init() {
	cmd.FlagBuildpacksDir(&b.buildpacksDir)
	cmd.FlagGroupPath(&b.groupPath)
	cmd.FlagPlanPath(&b.planPath)
	cmd.FlagLayersDir(&b.layersDir)
	cmd.FlagAppDir(&b.appDir)
	cmd.FlagPlatformDir(&b.platformDir)
	cmd.FlagStackGroupPath(&b.stackGroupPath)
}

func (b *buildCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeInvalidArgs, "parse arguments")
	}
	return nil
}

func (b *buildCmd) Privileges() error {
	// builder should never be run with privileges if there aren't any stack buildpacks
	if b.stackGroupPath == "" && priv.IsPrivileged() {
		return cmd.FailErr(errors.New("refusing to run as root"), "build")
	}
	return nil
}

func (b *buildCmd) Exec() error {
	group, stackGroup, plan, err := b.readData()
	if err != nil {
		return err
	}

	return b.buildAll(group, stackGroup, plan)
}

func (ba buildArgs) buildAll(group, stackGroup lifecycle.BuildpackGroup, plan lifecycle.BuildPlan) error {
	if err := verifyBuildpackApis(group); err != nil {
		return err
	}

	if err := verifyBuildpackApis(stackGroup); err != nil {
		return err
	}

	builder, err := ba.createBuilder(group, stackGroup, plan)
	if err != nil {
		return err
	}

	if err = ba.stackBuild(builder); err != nil {
		return err
	}

	if err = ba.build(builder); err != nil {
		return err
	}
	return nil
}

func (ba buildArgs) stackBuild(builder *lifecycle.Builder) error {
	// run stack buildpacks as root
	_, err := builder.StackBuild()
	if err != nil {
		if err, ok := err.(*lifecycle.Error); ok {
			if err.Type == lifecycle.ErrTypeBuildpack {
				return cmd.FailErrCode(err.Cause(), cmd.CodeFailedBuildWithErrors, "stack-build")
			}
		}
		return cmd.FailErrCode(err, cmd.CodeBuildError, "stack-build")
	}
	return nil
}

func (ba buildArgs) build(builder *lifecycle.Builder) error {
	// drop back to non-root user to run buildpacks
	if err := priv.RunAs(ba.uid, ba.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", ba.uid, ba.gid))
	}
	if err := priv.SetEnvironmentForUser(ba.uid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("set environment for user %d", ba.uid))
	}
	md, err := builder.Build()
	if err != nil {
		if err, ok := err.(*lifecycle.Error); ok {
			if err.Type == lifecycle.ErrTypeBuildpack {
				return cmd.FailErrCode(err.Cause(), cmd.CodeFailedBuildWithErrors, "build")
			}
		}
		return cmd.FailErrCode(err, cmd.CodeBuildError, "build")
	}
	if err := lifecycle.WriteTOML(launch.GetMetadataFilePath(ba.layersDir), md); err != nil {
		return cmd.FailErr(err, "write build metadata")
	}
	return nil
}

func (ba buildArgs) createBuilder(group, stackGroup lifecycle.BuildpackGroup, plan lifecycle.BuildPlan) (*lifecycle.Builder, error) {
	stackSnapshotter, err := snapshot.NewKanikoSnapshotter("/")
	if err != nil {
		return &lifecycle.Builder{}, err
	}

	return &lifecycle.Builder{
		AppDir:        ba.appDir,
		LayersDir:     ba.layersDir,
		PlatformDir:   ba.platformDir,
		BuildpacksDir: ba.buildpacksDir,
		PlatformAPI:   api.MustParse(ba.platformAPI),
		Env:           env.NewBuildEnv(os.Environ()),
		Group:         group,
		StackGroup:    stackGroup,
		Plan:          plan,
		Out:           log.New(os.Stdout, "", 0),
		Err:           log.New(os.Stderr, "", 0),
		Snapshotter:   stackSnapshotter,
	}, nil
}

func (b *buildCmd) readData() (lifecycle.BuildpackGroup, lifecycle.BuildpackGroup, lifecycle.BuildPlan, error) {
	group := lifecycle.BuildpackGroup{}
	if _, err := os.Stat(b.groupPath); err == nil {
		group, err = lifecycle.ReadGroup(b.groupPath)
		if err != nil {
			return lifecycle.BuildpackGroup{}, lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read buildpack group")
		}
	}

	stackGroup := lifecycle.BuildpackGroup{}
	if _, err := os.Stat(b.stackGroupPath); err == nil {
		stackGroup, err = lifecycle.ReadGroup(b.stackGroupPath)
		if err != nil {
			return group, lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read stack buildpack group")
		}
	}

	var plan lifecycle.BuildPlan
	if _, err := toml.DecodeFile(b.planPath, &plan); err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "parse detect plan")
	}
	return group, stackGroup, plan, nil
}
