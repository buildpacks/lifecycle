package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/metadata"
)

var (
	buildpacksDir string
	groupPath     string
	planPath      string
	layersDir     string
	appDir        string
	platformDir   string
	printVersion  bool
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagPlanPath(&planPath)
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagPlatformDir(&platformDir)
	cmd.FlagVersion(&printVersion)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()

	if printVersion {
		cmd.ExitWithVersion()
	}

	if flag.NArg() != 0 {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments"))
	}
	cmd.Exit(build())
}

func build() error {
	group, err := lifecycle.ReadGroup(groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	var plan lifecycle.DetectPlan
	if _, err := toml.DecodeFile(planPath, &plan); err != nil {
		return cmd.FailErr(err, "parse detect plan")
	}

	env := &lifecycle.Env{
		Getenv:  os.Getenv,
		Setenv:  os.Setenv,
		Environ: os.Environ,
		Map:     lifecycle.POSIXBuildEnv,
	}

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

	metadataPath := metadata.MetadataFilePath(layersDir)
	if err := lifecycle.WriteTOML(metadataPath, md); err != nil {
		return cmd.FailErr(err, "write metadata")
	}
	return nil
}
