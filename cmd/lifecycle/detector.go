package main

import (
	"errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type detectCmd struct {
	*platform.Platform
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (d *detectCmd) DefineFlags() {
	switch {
	case d.PlatformAPI.AtLeast("0.10"):
		cli.FlagAnalyzedPath(&d.AnalyzedPath)
		cli.FlagAppDir(&d.AppDir)
		cli.FlagBuildpacksDir(&d.BuildpacksDir)
		cli.FlagExtensionsDir(&d.ExtensionsDir)
		cli.FlagGeneratedDir(&d.GeneratedDir)
		cli.FlagGroupPath(&d.GroupPath)
		cli.FlagLayersDir(&d.LayersDir)
		cli.FlagOrderPath(&d.OrderPath)
		cli.FlagPlanPath(&d.PlanPath)
		cli.FlagPlatformDir(&d.PlatformDir)
	default:
		cli.FlagAppDir(&d.AppDir)
		cli.FlagBuildpacksDir(&d.BuildpacksDir)
		cli.FlagGroupPath(&d.GroupPath)
		cli.FlagOrderPath(&d.OrderPath)
		cli.FlagLayersDir(&d.LayersDir)
		cli.FlagPlanPath(&d.PlanPath)
		cli.FlagPlatformDir(&d.PlatformDir)
		cli.FlagBuildConfigDir(&d.BuildConfigDir)
	}
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
	detectorFactory := lifecycle.NewDetectorFactory(
		d.PlatformAPI,
		&cmd.BuildpackAPIVerifier{},
		lifecycle.NewConfigHandler(),
		dirStore,
	)
	amd, err := platform.ReadAnalyzed(d.AnalyzedPath, cmd.DefaultLogger)
	if err != nil {
		return unwrapErrorFailWithMessage(err, "reading analyzed.toml")
	}
	detector, err := detectorFactory.NewDetector(
		amd,
		d.AppDir,
		d.BuildConfigDir,
		d.OrderPath,
		d.PlatformDir,
		cmd.DefaultLogger,
	)
	if err != nil {
		return unwrapErrorFailWithMessage(err, "initialize detector")
	}
	if detector.HasExtensions {
		if err = platform.GuardExperimental(platform.FeatureDockerfiles, cmd.DefaultLogger); err != nil {
			return err
		}
	}
	group, plan, err := doDetect(detector, d.Platform)
	if err != nil {
		return err // pass through error
	}
	if group.HasExtensions() {
		generatorFactory := lifecycle.NewGeneratorFactory(
			&cmd.BuildpackAPIVerifier{},
			dirStore,
		)
		var generator *lifecycle.Generator
		generator, err = generatorFactory.NewGenerator(
			d.AppDir,
			d.BuildConfigDir,
			group.GroupExtensions,
			d.GeneratedDir,
			plan,
			d.PlatformDir,
			cmd.Stdout, cmd.Stderr,
			cmd.DefaultLogger,
		)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize generator")
		}
		var result lifecycle.GenerateResult
		result, err = generator.Generate()
		if err != nil {
			return d.unwrapGenerateFail(err)
		}
		// was a custom run image configured?
		if result.RunImage != "" {
			cmd.DefaultLogger.Debug("Updating analyzed metadata with new runImage")
			detector.AnalyzeMD.RunImage = platform.RunImage{Reference: result.RunImage}
			if err = d.writeGenerateData(detector.AnalyzeMD); err != nil {
				return err
			}
			cmd.DefaultLogger.Debugf("Updated analyzed metadata with new runImage '%s'", result.RunImage)
		}
		// was the build plan updated?
		if result.UsePlan {
			plan = result.Plan
		}
	}
	return d.writeDetectData(group, plan)
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

func doDetect(detector *lifecycle.Detector, p *platform.Platform) (buildpack.Group, platform.BuildPlan, error) {
	group, plan, err := detector.Detect()
	if err != nil {
		switch err := err.(type) {
		case *buildpack.Error:
			switch err.Type {
			case buildpack.ErrTypeFailedDetection:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				cmd.DefaultLogger.Error("Please check that you are running against the correct path.")
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, p.CodeFor(platform.FailedDetect), "detect")
			case buildpack.ErrTypeBuildpack:
				cmd.DefaultLogger.Error("No buildpack groups passed detection.")
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, p.CodeFor(platform.FailedDetectWithErrors), "detect")
			default:
				return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, p.CodeFor(platform.DetectError), "detect")
			}
		default:
			return buildpack.Group{}, platform.BuildPlan{}, cmd.FailErrCode(err, p.CodeFor(platform.DetectError), "detect")
		}
	}
	return group, plan, nil
}

func (d *detectCmd) writeDetectData(group buildpack.Group, plan platform.BuildPlan) error {
	if err := encoding.WriteTOML(d.GroupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}
	if err := encoding.WriteTOML(d.PlanPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}
	return nil
}

// writeGenerateData re-outputs the analyzedMD that we read previously, but now we've added the RunImage, if a custom runImage was configured
func (d *detectCmd) writeGenerateData(analyzedMD platform.AnalyzedMetadata) error {
	if err := analyzedMD.WriteTOML(d.AnalyzedPath); err != nil {
		return cmd.FailErr(err, "write analyzed metadata")
	}
	return nil
}
