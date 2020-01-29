package main

import (
	"flag"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

type buildCmd struct {
	buildpacksDir string
	groupPath     string
	planPath      string
	layersDir     string
	appDir        string
	platformDir   string
}

func (b *buildCmd) Flags() {
	cmd.FlagBuildpacksDir(&b.buildpacksDir)
	cmd.FlagGroupPath(&b.groupPath)
	cmd.FlagPlanPath(&b.planPath)
	cmd.FlagLayersDir(&b.layersDir)
	cmd.FlagAppDir(&b.appDir)
	cmd.FlagPlatformDir(&b.platformDir)

	flag.Parse()
}

func (b *buildCmd) Args() error {
	if flag.NArg() != 0 {
		return cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments")
	}
	return nil
}

func (b *buildCmd) Exec() error {
	group, err := lifecycle.ReadGroup(b.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	var plan lifecycle.BuildPlan
	if _, err := toml.DecodeFile(b.planPath, &plan); err != nil {
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
		AppDir:        b.appDir,
		LayersDir:     b.layersDir,
		PlatformDir:   b.platformDir,
		BuildpacksDir: b.buildpacksDir,
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

	if err := lifecycle.WriteTOML(lifecycle.MetadataFilePath(b.layersDir), md); err != nil {
		return cmd.FailErr(err, "write metadata")
	}
	return nil
}
