package main

import (
	"errors"
	"fmt"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/internal/extend/kaniko"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/config"
	"github.com/buildpacks/lifecycle/platform/exit"
	"github.com/buildpacks/lifecycle/platform/exit/fail"
	"github.com/buildpacks/lifecycle/priv"
)

type extendCmd struct {
	*platform.Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (e *extendCmd) DefineFlags() {
	if e.PlatformAPI.AtLeast("0.12") {
		cli.FlagExtendKind(&e.ExtendKind)
		cli.FlagExtendedDir(&e.ExtendedDir)
	}
	cli.FlagAnalyzedPath(&e.AnalyzedPath)
	cli.FlagAppDir(&e.AppDir)
	cli.FlagBuildpacksDir(&e.BuildpacksDir)
	cli.FlagGID(&e.GID)
	cli.FlagGeneratedDir(&e.GeneratedDir)
	cli.FlagGroupPath(&e.GroupPath)
	cli.FlagKanikoCacheTTL(&e.KanikoCacheTTL)
	cli.FlagLayersDir(&e.LayersDir)
	cli.FlagPlanPath(&e.PlanPath)
	cli.FlagPlatformDir(&e.PlatformDir)
	cli.FlagUID(&e.UID)
}

// Args validates arguments and flags, and fills in default values.
func (e *extendCmd) Args(nargs int, _ []string) error {
	if nargs != 0 {
		return exit.ErrorFromErrAndCode(errors.New("received unexpected arguments"), exit.CodeForInvalidArgs, "parse arguments")
	}
	if err := platform.ResolveInputs(platform.Extend, e.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return exit.ErrorFromErrAndCode(err, exit.CodeForInvalidArgs, "resolve inputs")
	}
	return nil
}

func (e *extendCmd) Privileges() error {
	return nil
}

func (e *extendCmd) Exec() error {
	extenderFactory := lifecycle.NewExtenderFactory(&config.BuildpackAPIVerifier{}, lifecycle.NewConfigHandler())
	applier, err := kaniko.NewDockerfileApplier()
	if err != nil {
		return err
	}
	extender, err := extenderFactory.NewExtender(
		e.AnalyzedPath,
		e.AppDir,
		e.ExtendedDir,
		e.GeneratedDir,
		e.GroupPath,
		e.LayersDir,
		e.PlatformDir,
		e.KanikoCacheTTL,
		applier,
		e.ExtendKind,
		cmd.DefaultLogger,
	)
	if err != nil {
		return unwrapErrorFailWithMessage(err, "initialize extender")
	}
	switch e.ExtendKind {
	case buildpack.DockerfileKindBuild:
		if err = extender.Extend(e.ExtendKind, cmd.DefaultLogger); err != nil {
			return exit.ErrorFromErrAndCode(err, e.CodeFor(fail.ExtendError), "extend build image")
		}
		if err = priv.EnsureOwner(e.UID, e.GID, e.LayersDir); err != nil {
			return exit.ErrorFromErr(err, "chown volumes")
		}
		if err = priv.RunAs(e.UID, e.GID); err != nil {
			return exit.ErrorFromErr(err, fmt.Sprintf("exec as user %d:%d", e.UID, e.GID))
		}
		if err = priv.SetEnvironmentForUser(e.UID); err != nil {
			return exit.ErrorFromErr(err, fmt.Sprintf("set environment for user %d", e.UID))
		}
		buildCmd := buildCmd{Platform: e.Platform}
		if err = buildCmd.Privileges(); err != nil {
			return err
		}
		return buildCmd.Exec()
	case buildpack.DockerfileKindRun:
		if err = extender.Extend(e.ExtendKind, cmd.DefaultLogger); err != nil {
			return exit.ErrorFromErrAndCode(err, e.CodeFor(fail.ExtendError), "extend run image")
		}
	default:
		return exit.ErrorFromErrAndCode(err, exit.CodeForInvalidArgs)
	}
	return nil
}
