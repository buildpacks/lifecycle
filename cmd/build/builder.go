package main

import (
	"flag"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

type buildFlags struct {
	buildpacksDir string
	groupPath     string
	planPath      string
	layersDir     string
	appDir        string
	platformDir   string
}

func parseBuildFlags() (buildFlags, error) {
	f := buildFlags{}
	cmd.FlagBuildpacksDir(&f.buildpacksDir)
	cmd.FlagGroupPath(&f.groupPath)
	cmd.FlagPlanPath(&f.planPath)
	cmd.FlagLayersDir(&f.layersDir)
	cmd.FlagAppDir(&f.appDir)
	cmd.FlagPlatformDir(&f.platformDir)

	flag.Parse()
	commonFlags()

	if flag.NArg() != 0 {
		return buildFlags{}, cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments")
	}

	return f, nil
}

func builder(f buildFlags) error {
	group, err := lifecycle.ReadGroup(f.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	var plan lifecycle.BuildPlan
	if _, err := toml.DecodeFile(f.planPath, &plan); err != nil {
		return cmd.FailErr(err, "parse detect plan")
	}

	return build(f.appDir, f.layersDir, f.platformDir, f.buildpacksDir, group, plan)
}

func build(appDir, layersDir, platformDir, buildpacksDir string, group lifecycle.BuildpackGroup, plan lifecycle.BuildPlan) error {
	builder := &lifecycle.Builder{
		AppDir:        appDir,
		LayersDir:     layersDir,
		PlatformDir:   platformDir,
		BuildpacksDir: buildpacksDir,
		Env:           env,
		Group:         group,
		Plan:          plan,
		Out:           log.New(os.Stdout, "", 0),
		Err:           log.New(os.Stderr, "", 0),
	}

	md, err := builder.Build()
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild, "build")
	}

	if err := lifecycle.WriteTOML(lifecycle.MetadataFilePath(layersDir), md); err != nil {
		return cmd.FailErr(err, "write metadata")
	}
	return nil
}
