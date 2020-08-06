package main

import (
	"fmt"
	"log"
	"os"

	"github.com/pkg/errors"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle"
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
	uid, gid       int

	buildArgs
}

type buildArgs struct {
	// inputs needed when run by creator
	buildpacksDir string
	layersDir     string
	appDir        string
	platformDir   string
}

func (b *buildCmd) Init() {
	cmd.FlagBuildpacksDir(&b.buildpacksDir)
	cmd.FlagGroupPath(&b.groupPath)
	cmd.FlagStackGroupPath(&b.stackGroupPath)
	cmd.FlagPlanPath(&b.planPath)
	cmd.FlagLayersDir(&b.layersDir)
	cmd.FlagAppDir(&b.appDir)
	cmd.FlagPlatformDir(&b.platformDir)
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
	if err := verifyBuildpackApis(group); err != nil {
		return err
	}

	// run stack buildpacks as root
	stackSnapshotter, err := snapshot.NewKanikoSnapshotter("/")
	if err != nil {
		return err
	}
	if err = b.build(stackGroup, plan, stackSnapshotter.RootDir, stackSnapshotter); err != nil {
		return errors.Wrap(err, "running stack buildpacks")
	}

	// drop back to non-root user to run buildpacks
	if err := priv.RunAs(b.uid, b.gid); err != nil {
		return errors.Wrap(err, fmt.Sprintf("exec as user %d:%d", b.uid, b.gid))
	}
	if err := priv.SetEnvironmentForUser(b.uid); err != nil {
		return errors.Wrap(err, fmt.Sprintf("set environment for user %d", b.uid))
	}
	if err = b.build(group, plan, b.appDir, &lifecycle.NoopSnapshotter{}); err != nil {
		return errors.Wrap(err, "running buildpacks")
	}
	return nil
}

func (ba buildArgs) build(group lifecycle.BuildpackGroup, plan lifecycle.BuildPlan, baseDir string, snapshotter lifecycle.LayerSnapshotter) error {
	builder := &lifecycle.Builder{
		AppDir:        baseDir,
		LayersDir:     ba.layersDir,
		PlatformDir:   ba.platformDir,
		BuildpacksDir: ba.buildpacksDir,
		Env:           env.NewBuildEnv(os.Environ()),
		Group:         group,
		Plan:          plan,
		Out:           log.New(os.Stdout, "", 0),
		Err:           log.New(os.Stderr, "", 0),
		Snapshotter:   snapshotter,
	}
	md, err := builder.Build()
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild, "build")
	}

	if err := lifecycle.WriteTOML(launch.GetMetadataFilePath(ba.layersDir), md); err != nil {
		return cmd.FailErr(err, "write build metadata")
	}
	return nil
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
		return group, stackGroup, lifecycle.BuildPlan{}, cmd.FailErr(err, "parse detect plan")
	}
	return group, stackGroup, plan, nil
}
