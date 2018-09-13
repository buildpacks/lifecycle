package main

import (
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle"
	"flag"
)

var (
	launchDir  string
)

const knativeBuildHomeDir = "/builder/home"
const knativeWorkspaceDir = "/workspace"

func init(){
	flag.StringVar(&launchDir, "launch", knativeWorkspaceDir, "path to launch directory")
}


func main() {
	flag.Parse()
	if launchDir == "" {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "empty launch dir"))
	}
	if err := lifecycle.SetupKnativeLaunchDir(launchDir); err != nil {
		cmd.Exit(cmd.FailCode(cmd.CodeFailed, "moving app dir"))
	}
	cmd.Exit(lifecycle.ChownDirs(launchDir, knativeBuildHomeDir))
}