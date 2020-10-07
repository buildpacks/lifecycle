package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"

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
	privGroupPath string
	groupPath     string
	planPath      string
	buildArgs
}

type buildArgs struct {
	// inputs needed when run by creator
	uid, gid           int
	buildpacksDir      string
	layersDir          string
	appDir             string
	platformDir        string
	platformAPI        string
	stackBuildpacksDir string
}

func (b *buildCmd) Init() {
	cmd.FlagBuildpacksDir(&b.buildpacksDir)
	cmd.FlagGID(&b.gid)
	cmd.FlagGroupPath(&b.groupPath)
	cmd.FlagPlanPath(&b.planPath)
	cmd.FlagLayersDir(&b.layersDir)
	cmd.FlagAppDir(&b.appDir)
	cmd.FlagPlatformDir(&b.platformDir)
	cmd.FlagStackBuildpacksDir(&b.stackBuildpacksDir)
	cmd.FlagPrivilegedGroupPath(&b.privGroupPath)
	cmd.FlagUID(&b.uid)
}

func (b *buildCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeInvalidArgs, "parse arguments")
	}
	return nil
}

func (b *buildCmd) Privileges() error {
	_, privGroup, _, err := b.readData()
	if err != nil {
		return err
	}

	hasPrivGroups := len(privGroup.Group) > 0

	// builder should never be run with privileges if there aren't any stack buildpacks
	if !hasPrivGroups && priv.IsPrivileged() {
		return cmd.FailErr(errors.New("refusing to run as root"), "build")
	} else if hasPrivGroups && !priv.IsPrivileged() {
		return cmd.FailErr(errors.New("must run as root"), "build")
	}
	return nil
}

func (b *buildCmd) Exec() error {
	group, privGroup, plan, err := b.readData()
	if err != nil {
		return err
	}

	if err := verifyBuildpackApis(group, privGroup); err != nil {
		return err
	}

	if len(privGroup.Group) == 0 {
		return b.build(group, plan)
	}
	return b.buildWithReexec(group, privGroup, plan)
}

func (ba buildArgs) buildAll(group, privGroup lifecycle.BuildpackGroup, plan lifecycle.BuildPlan, rootuid, rootgid, uid, gid int) error {
	if len(privGroup.Group) > 0 {
		if err := priv.RunAsEffective(rootuid, rootgid); err != nil {
			return err
		}
		if err := ba.stackBuild(privGroup, plan); err != nil {
			return err
		}
		if err := priv.RunAsEffective(uid, gid); err != nil {
			return err
		}
	}

	return ba.build(group, plan)
}

func (ba buildArgs) stackBuild(privGroup lifecycle.BuildpackGroup, plan lifecycle.BuildPlan) error {
	stackSnapshotter, err := snapshot.NewKanikoSnapshotter("/", ba.layersDir, ba.platformDir)
	if err != nil {
		return err
	}

	workspaceDir, err := ioutil.TempDir("", "stack-workspace")
	if err != nil {
		return err
	}

	builder, err := ba.createBuilder(privGroup, plan)
	if err != nil {
		return err
	}

	builder.Snapshotter = stackSnapshotter
	builder.AppDir = workspaceDir
	builder.BuildpacksDir = ba.stackBuildpacksDir

	_, err = ba.execBuild(builder)
	return err
}

func (ba buildArgs) build(group lifecycle.BuildpackGroup, plan lifecycle.BuildPlan) error {
	builder, err := ba.createBuilder(group, plan)
	if err != nil {
		return err
	}
	md, err := ba.execBuild(builder)
	if err != nil {
		return err
	}

	if err := lifecycle.WriteTOML(launch.GetMetadataFilePath(ba.layersDir), md); err != nil {
		return cmd.FailErr(err, "write build metadata")
	}
	return nil
}

func (ba buildArgs) buildWithReexec(group, privGroup lifecycle.BuildpackGroup, plan lifecycle.BuildPlan) error {
	err := ba.stackBuild(privGroup, plan)
	if err != nil {
		return err
	}

	if len(group.Group) > 0 {
		if err = ba.buildAsSubProcess(); err != nil {
			return err
		}
	}
	return nil
}

func (ba buildArgs) buildAsSubProcess() error {
	if err := priv.RunAs(ba.uid, ba.gid); err != nil {
		cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", ba.uid, ba.gid))
	}

	// we need to handle arg <value> and arg=<value> to remove the argument on re-exec
	var args = os.Args[1:]
	var sgPathIndex int
	var sgPathIndexLength int = 2
	for i, arg := range args {
		if strings.HasPrefix(arg, cmd.FlagNamePrivilegedGroupPath) {
			sgPathIndex = i
			if strings.Contains(arg, "=") {
				sgPathIndexLength = 1
			}
			break
		}
	}

	if sgPathIndex > 0 {
		args = append(args[:sgPathIndex], args[sgPathIndex+sgPathIndexLength:]...)
	}

	// explicitly omit Privileged Group to skip Stack Buildpacks on this execution
	args = append(args, fmt.Sprintf("-%s", cmd.FlagNamePrivilegedGroupPath), "")

	c := exec.Command(
		os.Args[0],
		args...,
	)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c.Run()
}

func (ba buildArgs) execBuild(builder *lifecycle.Builder) (*lifecycle.BuildMetadata, error) {
	md, err := builder.Build()
	if err != nil {
		if err, ok := err.(*lifecycle.Error); ok {
			if err.Type == lifecycle.ErrTypeBuildpack {
				return nil, cmd.FailErrCode(err.Cause(), cmd.CodeFailedBuildWithErrors, "build")
			}
		}
		return nil, cmd.FailErrCode(err, cmd.CodeBuildError, "build")
	}

	return md, nil
}

func (ba buildArgs) createBuilder(group lifecycle.BuildpackGroup, plan lifecycle.BuildPlan) (*lifecycle.Builder, error) {
	return &lifecycle.Builder{
		AppDir:        ba.appDir,
		LayersDir:     ba.layersDir,
		PlatformDir:   ba.platformDir,
		BuildpacksDir: ba.buildpacksDir,
		PlatformAPI:   api.MustParse(ba.platformAPI),
		Env:           env.NewBuildEnv(os.Environ()),
		Group:         group,
		Plan:          plan,
		Out:           log.New(os.Stdout, "", 0),
		Err:           log.New(os.Stderr, "", 0),
	}, nil
}

func (b *buildCmd) readData() (lifecycle.BuildpackGroup, lifecycle.BuildpackGroup, lifecycle.BuildPlan, error) {
	group, privGroup, err := lifecycle.ReadGroups(b.groupPath, b.privGroupPath)
	if err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "read buildpack group")
	}

	var plan lifecycle.BuildPlan
	if _, err := toml.DecodeFile(b.planPath, &plan); err != nil {
		return lifecycle.BuildpackGroup{}, lifecycle.BuildpackGroup{}, lifecycle.BuildPlan{}, cmd.FailErr(err, "parse detect plan")
	}
	return group, privGroup, plan, nil
}
