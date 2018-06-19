package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/sclevine/packs"

	"github.com/sclevine/lifecycle"
)

var (
	buildpackPath string
	groupPath     string
	infoPath      string
	metadataPath  string
)

func init() {
	packs.InputBPPath(&buildpackPath)
	packs.InputBPGroupPath(&groupPath)
	packs.InputDetectInfoPath(&infoPath)

	packs.InputMetadataPath(&metadataPath)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || groupPath == "" || infoPath == "" || metadataPath == "" {
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse arguments"))
	}
	packs.Exit(build())
}

func build() error {
	buildpacks, err := lifecycle.NewBuildpackMap(buildpackPath)
	if err != nil {
		return packs.FailErr(err, "read buildpack directory")
	}
	var group struct {
		Buildpacks []string
	}
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return packs.FailErr(err, "read group")
	}
	info, err := ioutil.ReadFile(infoPath)
	if err != nil {
		return packs.FailErr(err, "read detect info")
	}
	builder := &lifecycle.Builder{
		PlatformDir: lifecycle.DefaultPlatformDir,
		Buildpacks:  buildpacks.FromList(group.Buildpacks),
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
		lifecycle.DefaultAppDir,
		lifecycle.DefaultLaunchDir,
		lifecycle.DefaultCacheDir,
		env,
	)
	if err != nil {
		return packs.FailErrCode(err, packs.CodeFailedBuild)
	}
	mdFile, err := os.Create(metadataPath)
	if err != nil {
		return packs.FailErr(err, "create metadata file")
	}
	defer mdFile.Close()
	if err := json.NewEncoder(mdFile).Encode(metadata); err != nil {
		return packs.FailErr(err, "write metadata")
	}
	return nil
}
