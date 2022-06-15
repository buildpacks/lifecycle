package main

import (
	"errors"
	"io/ioutil"
	"strings"

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
		cmd.FlagGeneratedPath(&d.GeneratedPath)
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
		return cmd.FailErrCode(errors.New("received unexpected arguments"), cmd.CodeInvalidArgs, "parse arguments")
	}

	var err error
	d.DetectInputs, err = d.platform.ResolveDetect(d.DetectInputs)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeInvalidArgs, "resolve inputs")
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
		&cmd.APIVerifier{},
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
	group, plan, err := doDetect(detector, d.platform)
	if err != nil {
		return err // pass through error
	}
	if group.HasExtensions() {
		generatorFactory := lifecycle.NewGeneratorFactory(
			&cmd.APIVerifier{},
			dirStore,
		)
		generator, err := generatorFactory.NewGenerator(
			d.AppDir,
			group,
			d.OutputDir,
			plan,
			d.PlatformDir,
			cmd.DefaultLogger,
		)
		if err != nil {
			return unwrapErrorFailWithMessage(err, "initialize generator")
		}
		generated, err := generator.Generate()
		if err != nil {
			return d.unwrapGenerateFail(err)
		}
		analyzed, err := d.updateAnalyzed(generated.Dockerfiles)
		if err != nil {
			return err
		}
		if err := d.writeGenerateData(*generated, analyzed); err != nil {
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

func (d *detectCmd) updateAnalyzed(dockerfiles []buildpack.Dockerfile) (platform.AnalyzedMetadata, error) {
	// TODO: this function should move to the lifecycle package and be tested
	lastDockerfile := dockerfiles[len(dockerfiles)-1]
	contents, err := ioutil.ReadFile(lastDockerfile.Path)
	if err != nil {
		return platform.AnalyzedMetadata{}, err
	}
	parts := strings.Split(string(contents), " ") // TODO: use proper dockerfile parser instead (see kaniko)
	if len(parts) != 2 || parts[0] != "FROM" {
		return platform.AnalyzedMetadata{}, errors.New("failed to parse dockerfile, expected format 'FROM <image>'")
	}
	newRunImage := strings.TrimSpace(parts[1])
	analyzedMD, err := parseAnalyzedMD(cmd.DefaultLogger, d.AnalyzedPath)
	if err != nil {
		return platform.AnalyzedMetadata{}, err
	}
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: newRunImage}
	return analyzedMD, nil
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

func (d *detectCmd) writeGenerateData(generated platform.GeneratedMetadata, analyzedMD platform.AnalyzedMetadata) error {
	if err := encoding.WriteTOML(d.GeneratedPath, generated); err != nil {
		return cmd.FailErr(err, "write generated metadata")
	}
	if err := encoding.WriteTOML(d.AnalyzedPath, analyzedMD); err != nil {
		return cmd.FailErr(err, "write analyzed metadata")
	}
	return nil
}
