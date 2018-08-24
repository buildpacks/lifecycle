package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/buildpack/packs"

	"github.com/buildpack/lifecycle"
)

var (
	buildpackPath string
	groupPath     string
	infoPath      string
)

func init() {
	packs.InputBPPath(&buildpackPath)
	packs.InputBPGroupPath(&groupPath)
	packs.InputDetectInfoPath(&infoPath)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || groupPath == "" || infoPath == "" {
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse arguments"))
	}
	packs.Exit(build())
}

func build() error {
	buildpacks, err := lifecycle.NewBuildpackMap(buildpackPath)
	if err != nil {
		return packs.FailErr(err, "read buildpack directory")
	}
	group, err := buildpacks.ReadGroup(groupPath)
	if err != nil {
		return packs.FailErr(err, "read group")
	}

	info, err := ioutil.ReadFile(infoPath)
	if err != nil {
		return packs.FailErr(err, "read detect info")
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
		return packs.FailErrCode(err, packs.CodeFailedBuild)
	}

	metadataPath := filepath.Join(lifecycle.DefaultLaunchDir, "config", "metadata.toml")
	if err := lifecycle.WriteTOML(metadataPath, metadata); err != nil {
		return packs.FailErr(err, "write metadata")
	}
	return nil
}
