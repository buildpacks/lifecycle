package main

import (
	"errors"
	"path/filepath"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type detectCmd struct {
	platform Platform
	platform.DetectInputs
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (d *detectCmd) DefineFlags() {
	switch {
	case d.platform.API().AtLeast("0.10"):
		cmd.FlagAnalyzedPath(&d.AnalyzedPath)
		cmd.FlagAppDir(&d.AppDir)
		cmd.FlagBuildpacksDir(&d.BuildpacksDir)
		cmd.FlagExtensionsDir(&d.ExtensionsDir)
		cmd.FlagGroupPath(&d.GroupPath)
		cmd.FlagLayersDir(&d.LayersDir)
		cmd.FlagOrderPath(&d.OrderPath)
		cmd.FlagOutputDir(&d.OutputDir)
		cmd.FlagPlanPath(&d.PlanPath)
		cmd.FlagPlatformDir(&d.PlatformDir)
	default:
		cmd.FlagAppDir(&d.AppDir)
		cmd.FlagBuildpacksDir(&d.BuildpacksDir)
		cmd.FlagGroupPath(&d.GroupPath)
		cmd.FlagLayersDir(&d.LayersDir)
		cmd.FlagOrderPath(&d.OrderPath)
		cmd.FlagPlanPath(&d.PlanPath)
		cmd.FlagPlatformDir(&d.PlatformDir)
	}
}

// Args validates arguments and flags, and fills in default values.
func (d *detectCmd) Args(nargs int, args []string) error {
	if nargs != 0 {
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeForInvalidArgs, "parse arguments")
	}

	var err error
	d.DetectInputs, err = d.platform.ResolveDetect(d.DetectInputs)
	if err != nil {
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
	dirStore, err := platform.NewDirStore(d.BuildpacksDir, d.ExtensionsDir)
	if err != nil {
		return err
	}
	detectorFactory := lifecycle.NewDetectorFactory(
		d.platform.API(),
		&cmd.BuildpackAPIVerifier{},
		lifecycle.NewConfigHandler(),
		dirStore,
	)
	detector, err := detectorFactory.NewDetector(
		d.AppDir,
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
	group, plan, err := doDetect(detector, d.platform)
	if err != nil {
		return err // pass through error
	}
	if group.HasExtensions() {
		generatorFactory := lifecycle.NewGeneratorFactory(
			&cmd.BuildpackAPIVerifier{},
			dirStore,
		)
		generator, err := generatorFactory.NewGenerator(
			d.AppDir,
			group.GroupExtensions,
			filepath.Join(d.OutputDir, "generated"),
			plan,
			d.PlatformDir,
			cmd.Stdout, cmd.Stderr,
			cmd.DefaultLogger,
		)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize generator")
		}
		err = generator.Generate()
		if err != nil {
			return d.unwrapGenerateFail(err)
		}
		extenderFactory := lifecycle.NewExtenderFactory(
			&cmd.BuildpackAPIVerifier{},
			dirStore,
		)
		extender, err := extenderFactory.NewExtender(
			group.GroupExtensions,
			generator.OutputDir,
			cmd.DefaultLogger,
		)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize extender")
		}
		newRunImage, err := extender.LastRunImage()
		if err != nil {
			return cmd.FailErr(err, "determine last run image")
		}
		analyzedMD, err := parseAnalyzedMD(cmd.DefaultLogger, d.AnalyzedPath)
		if err != nil {
			return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "parse analyzed metadata")
		}
		analyzedMD.RunImage = &platform.ImageIdentifier{Reference: newRunImage}
		if err := d.writeGenerateData(analyzedMD); err != nil {
			return err
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
			return cmd.FailErrCode(err.Cause(), d.platform.CodeFor(platform.FailedGenerateWithErrors), "build")
		}
	}
	return cmd.FailErrCode(err, d.platform.CodeFor(platform.GenerateError), "build")
}

func doDetect(detector *lifecycle.Detector, p Platform) (buildpack.Group, platform.BuildPlan, error) {
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

func (d *detectCmd) writeGenerateData(analyzedMD platform.AnalyzedMetadata) error {
	if err := encoding.WriteTOML(d.AnalyzedPath, analyzedMD); err != nil {
		return cmd.FailErr(err, "write analyzed metadata")
	}
	return nil
}
