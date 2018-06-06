package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/sclevine/lifecycle"
	"github.com/sclevine/packs"
)

var (
	groupPath    string
	metadataPath string
)

func init() {
	packs.InputGroupPath(&groupPath)
	packs.InputMetadataPath(&metadataPath)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || groupPath == "" || metadataPath == "" {
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse arguments"))
	}
	packs.Exit(build())
}

func build() error {
	flag.Parse()
	groupFile, err := os.Open(groupPath)
	if err != nil {
		return packs.FailErr(err, "open group file")
	}
	defer groupFile.Close()
	var group lifecycle.BuildpackGroup
	if err := json.NewDecoder(groupFile).Decode(&group); err != nil {
		return packs.FailErr(err, "read group")
	}
	builder := &lifecycle.Builder{
		PlatformDir: lifecycle.DefaultPlatformDir,
		Buildpacks:  group,
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
