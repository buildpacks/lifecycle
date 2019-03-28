package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
)

var (
	buildpacksDir string
	groupPath     string
	planPath      string
	layersDir     string
	appDir        string
	platformDir   string
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagPlanPath(&planPath)
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagPlatformDir(&platformDir)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()
	if flag.NArg() != 0 {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments"))
	}
	cmd.Exit(build())
}

func build() error {
	buildpacks, err := lifecycle.NewBuildpackMap(buildpacksDir)
	if err != nil {
		return cmd.FailErr(err, "read buildpack directory")
	}
	group, err := buildpacks.ReadGroup(groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	var plan lifecycle.Plan
	if _, err := toml.DecodeFile(planPath, &plan); err != nil {
		return cmd.FailErr(err, "parse build plan")
	}

	env := &lifecycle.Env{
		Getenv:  os.Getenv,
		Setenv:  os.Setenv,
		Environ: os.Environ,
		Map:     lifecycle.POSIXBuildEnv,
	}
	builder := &lifecycle.Builder{
		PlatformDir: platformDir,
		LayersDir:   layersDir,
		AppDir:      appDir,
		Env:         env,
		Buildpacks:  group.Buildpacks,
		Plan:        plan,
		Out:         os.Stdout,
		Err:         os.Stderr,
	}

	metadata, err := builder.Build()
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	metadataPath := filepath.Join(layersDir, "config", "metadata.toml")
	if err := lifecycle.WriteTOML(metadataPath, metadata); err != nil {
		return cmd.FailErr(err, "write metadata")
	}
	return nil
}
