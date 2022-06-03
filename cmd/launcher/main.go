package main

import (
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/launcher/cli"
)

func main() {
	cmd.Exit(cli.RunLaunch())
}
