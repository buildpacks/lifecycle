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
	launchDir     string
	cacheDir      string
	platformDir   string
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagPlanPath(&planPath)
	cmd.FlagLaunchDir(&launchDir)
	cmd.FlagCacheDir(&cacheDir)
	cmd.FlagPlatformDir(&platformDir)
}

func main() {
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

	info, err := ioutil.ReadFile(planPath)
	if err != nil {
		return cmd.FailErr(err, "read build plan")
	}

	builder := &lifecycle.Builder{
		PlatformDir: platformDir,
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
		cacheDir,
		launchDir,
		env,
	)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	metadataPath := filepath.Join(launchDir, "config", "metadata.toml")
	if err := lifecycle.WriteTOML(metadataPath, metadata); err != nil {
		return cmd.FailErr(err, "write metadata")
	}
	return nil
}
