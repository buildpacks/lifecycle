package main

import (
	"flag"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
)

var (
	launchDir string
	uid       int
	gid       int
)

const knativeBuildHomeDir = "/builder/home"
const knativeWorkspaceDir = "/workspace"
const knativeCacheDir = "/cache"

func init() {
	flag.StringVar(&launchDir, "launch", knativeWorkspaceDir, "path to launch directory")
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
}

func main() {
	flag.Parse()
	if launchDir == "" {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "empty launch dir"))
	}
	if err := lifecycle.SetupKnativeLaunchDir(launchDir); err != nil {
		cmd.Exit(cmd.FailCode(cmd.CodeFailed, "moving app dir"))
	}
	cmd.Exit(lifecycle.ChownDirs(launchDir, knativeBuildHomeDir, knativeCacheDir, uid, gid))
}
