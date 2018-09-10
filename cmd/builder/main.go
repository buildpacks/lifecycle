package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
)

var (
	buildpacksDir string
	groupPath     string
	planPath      string
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagPlanPath(&planPath)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || groupPath == "" || planPath == "" {
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

	info, err := ioutil.ReadFile(planPath)
	if err != nil {
		return cmd.FailErr(err, "read build plan")
	}

	builder := &lifecycle.Builder{
		PlatformDir: lifecycle.DefaultPlatformDir,
		Buildpacks:  group.Buildpacks,
		In:          info,
		Out:         os.Stdout,
		Err:         os.Stderr,
	}
	env := &lifecycle.Env{
		Getenv:  os.Getenv,
		Setenv:  os.Setenv,
		Environ: os.Environ,
		Map:     lifecycle.POSIXBuildEnv,
	}
	metadata, err := builder.Build(
		lifecycle.DefaultCacheDir,
		lifecycle.DefaultLaunchDir,
		env,
	)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	metadataPath := filepath.Join(lifecycle.DefaultLaunchDir, "config", "metadata.toml")
	if err := lifecycle.WriteTOML(metadataPath, metadata); err != nil {
		return cmd.FailErr(err, "write metadata")
	}
	return nil
}
