package main

import (
	"errors"
	"fmt"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/extend/kaniko"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type extendCmd struct {
	*platform.Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (e *extendCmd) DefineFlags() {
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
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	if err := platform.ResolveInputs(platform.Extend, e.LifecycleInputs, &image.NopHandler{}, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	return nil
}

func (e *extendCmd) Privileges() error {
	return nil
}

func (e *extendCmd) Exec() error {
	extenderFactory := lifecycle.NewExtenderFactory(&cmd.BuildpackAPIVerifier{}, lifecycle.NewConfigHandler())
	extender, err := extenderFactory.NewExtender(
		e.AnalyzedPath,
		e.AppDir,
		e.GeneratedDir,
		e.GroupPath,
		e.LayersDir,
		e.PlatformDir,
		e.KanikoCacheTTL,
		kaniko.NewDockerfileApplier(cmd.DefaultLogger),
		cmd.DefaultLogger,
	)
	if err != nil {
		return unwrapErrorFailWithMessage(err, "initialize extender")
	}
	if err = extender.ExtendBuild(); err != nil {
		return cmd.FailErrCode(err, e.CodeFor(platform.ExtendError), "extend build image")
	}
	if err = priv.EnsureOwner(e.UID, e.GID, e.LayersDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err = priv.RunAs(e.UID, e.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", e.UID, e.GID))
	}
	if err = priv.SetEnvironmentForUser(e.UID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("set environment for user %d", e.UID))
	}
	buildCmd := buildCmd{Platform: e.Platform}
	if err = buildCmd.Privileges(); err != nil {
		return err
	}
	return buildCmd.Exec()
}
