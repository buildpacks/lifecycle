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

func build(f buildFlags) error {
	group, err := lifecycle.ReadGroup(f.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	var plan lifecycle.BuildPlan
	if _, err := toml.DecodeFile(f.planPath, &plan); err != nil {
		return cmd.FailErr(err, "parse detect plan")
	}

	env := &lifecycle.Env{
		LookupEnv: os.LookupEnv,
		Getenv:    os.Getenv,
		Setenv:    os.Setenv,
		Unsetenv:  os.Unsetenv,
		Environ:   os.Environ,
		Map:       lifecycle.POSIXBuildEnv,
	}

	builder := &lifecycle.Builder{
		AppDir:        f.appDir,
		LayersDir:     f.layersDir,
		PlatformDir:   f.platformDir,
		BuildpacksDir: f.buildpacksDir,
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

	if err := lifecycle.WriteTOML(lifecycle.MetadataFilePath(f.layersDir), md); err != nil {
		return cmd.FailErr(err, "write metadata")
	}
	return nil
}
