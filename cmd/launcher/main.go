package main

import (
	"flag"
	"os"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/packs"
)

var (
	metadataPath string
)

func init() {
	packs.InputMetadataPath(&metadataPath)
}

func main() {
	flag.Parse()
	if metadataPath == "" {
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse arguments"))
	}
	packs.Exit(launch())
}

func launch() error {
	defaultProcessType := "web"
	if v := os.Getenv("PACK_PROCESS_TYPE"); v != "" {
		defaultProcessType = v
	}

	var metadata lifecycle.BuildMetadata
	if _, err := toml.DecodeFile(metadataPath, &metadata); err != nil {
		return packs.FailErr(err, "read metadata")
	}

	launcher := &lifecycle.Launcher{
		DefaultProcessType: defaultProcessType,
		DefaultLaunchDir:   lifecycle.DefaultLaunchDir,
		DefaultAppDir:      lifecycle.DefaultAppDir,
		Processes:          metadata.Processes,
		Buildpacks:         metadata.Buildpacks,
		Exec:               syscall.Exec,
	}

	command := ""
	if flag.NArg() > 0 {
		command = strings.Join(flag.Args(), " ")
	}

	return launcher.Launch(os.Args[0], command)
}
