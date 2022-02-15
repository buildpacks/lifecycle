package main

import (
	"github.com/buildpacks/lifecycle/cmd"
)

func main() {
	platformAPI := cmd.EnvOrDefault(cmd.EnvPlatformAPI, cmd.DefaultPlatformAPI)
	if err := cmd.VerifyPlatformAPI(platformAPI); err != nil {
		cmd.Exit(err)
	}

	cmd.Run(&extendCmd{platformAPI: platformAPI}, false)
}
