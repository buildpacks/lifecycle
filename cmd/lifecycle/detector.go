package main

import (
	"errors"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	"github.com/buildpacks/lifecycle/priv"
)

type detectCmd struct {
	*platform.Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (d *detectCmd) DefineFlags() {
	if d.PlatformAPI.AtLeast("0.12") {
		cli.FlagRunPath(&d.RunPath)
	}
	if d.PlatformAPI.AtLeast("0.11") {
		cli.FlagBuildConfigDir(&d.BuildConfigDir)
	}
	if d.PlatformAPI.AtLeast("0.10") {
		cli.FlagAnalyzedPath(&d.AnalyzedPath)
		cli.FlagExtensionsDir(&d.ExtensionsDir)
		cli.FlagGeneratedDir(&d.GeneratedDir)
	}
	cli.FlagAppDir(&d.AppDir)
	cli.FlagBuildpacksDir(&d.BuildpacksDir)
	cli.FlagGroupPath(&d.GroupPath)
	cli.FlagLayersDir(&d.LayersDir)
	cli.FlagLogLevel(&d.LogLevel)
	cli.FlagNoColor(&d.NoColor)
	cli.FlagOrderPath(&d.OrderPath)
	cli.FlagPlanPath(&d.PlanPath)
	// TODO: platform version at least
	cli.FlagExecutionEnviornment(&d.ExecutionEnviornment)
}

// Args validates arguments and flags, and fills in default values.
func (d *detectCmd) Args(nargs int, _ []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	if err := platform.ResolveInputs(platform.Detect, d.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	return nil
}

func (d *detectCmd) Privileges() error {
	// detector should never be run with privileges
	if priv.IsPrivileged() {
		return cmd.FailErr(errors.New("refusing to run as root"), "detect")
	}
	return nil
}

func (d *detectCmd) Exec() error {
	dirStore := platform.NewDirStore(d.BuildpacksDir, d.ExtensionsDir)
	detectorFactory := phase.NewHermeticFactory(
		d.PlatformAPI,
		&cmd.BuildpackAPIVerifier{},
		files.NewHandler(),
		dirStore,
	)
	detector, err := detectorFactory.NewDetector(
		d.Inputs(),
		cmd.DefaultLogger,
	)
	if err != nil {
		return unwrapErrorFailWithMessage(err, "initialize detector")
	}
	if detector.HasExtensions && detector.PlatformAPI.LessThan("0.13") {
		if err = platform.GuardExperimental(platform.FeatureDockerfiles, cmd.DefaultLogger); err != nil {
			return err
		}
	}
	group, _, err := doDetect(detector, d.Platform)
	if err != nil {
		return err // pass through error
	}
	if group.HasExtensions() {
		generatorFactory := phase.NewHermeticFactory(
			d.PlatformAPI,
			&cmd.BuildpackAPIVerifier{},
			files.Handler,
			dirStore,
		)
		var generator *phase.Generator
		generator, err = generatorFactory.NewGenerator(
			d.Inputs(),
			cmd.Stdout, cmd.Stderr,
			cmd.DefaultLogger,
		)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize generator")
		}
		var result phase.GenerateResult
		result, err = generator.Generate()
		if err != nil {
			return d.unwrapGenerateFail(err)
		}
		if err := files.Handler.WriteAnalyzed(d.AnalyzedPath, &result.AnalyzedMD, cmd.DefaultLogger); err != nil {
			return err
		}
		if err := files.Handler.WritePlan(d.PlanPath, &result.Plan); err != nil {
			return err
		}
	}
	return nil
}

func unwrapErrorFailWithMessage(err error, msg string) error {
	errorFail, ok := err.(*cmd.ErrorFail)
	if ok {
		return errorFail
	}
	return cmd.FailErr(err, msg)
}

func (d *detectCmd) unwrapGenerateFail(err error) error {
	if err, ok := err.(*buildpack.Error); ok {
		if err.Type == buildpack.ErrTypeBuildpack {
			return cmd.FailErrCode(err.Cause(), d.CodeFor(platform.FailedGenerateWithErrors), "build")
		}
	}
	return cmd.FailErrCode(err, d.CodeFor(platform.GenerateError), "build")
}

func doDetect(detector *phase.Detector, p *platform.Platform) (buildpack.Group, files.Plan, error) {
	group, plan, err := detector.Detect()
	if err != nil {
		switch err := err.(type) {
		case *buildpack.Error:
			switch err.Type {
			case buildpack.ErrTypeFailedDetection:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				cmd.DefaultLogger.Error("Please check that you are running against the correct path.")
				return buildpack.Group{}, files.Plan{}, cmd.FailErrCode(err, p.CodeFor(platform.FailedDetect), "detect")
			case buildpack.ErrTypeBuildpack:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				return buildpack.Group{}, files.Plan{}, cmd.FailErrCode(err, p.CodeFor(platform.FailedDetectWithErrors), "detect")
			default:
				return buildpack.Group{}, files.Plan{}, cmd.FailErrCode(err, p.CodeFor(platform.DetectError), "detect")
			}
		default:
			return buildpack.Group{}, files.Plan{}, cmd.FailErrCode(err, p.CodeFor(platform.DetectError), "detect")
		}
	}
	if err := files.Handler.WriteGroup(p.GroupPath, &group); err != nil {
		return buildpack.Group{}, files.Plan{}, err
	}
	if err := files.Handler.WritePlan(p.PlanPath, &plan); err != nil {
		return buildpack.Group{}, files.Plan{}, err
	}
	return group, plan, nil
}
