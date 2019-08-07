package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/compat"
)

var (
	buildpacksDir string
	appDir        string
	platformDir   string
	orderPath     string
	groupPath     string
	planPath      string
	printVersion  bool
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagPlatformDir(&platformDir)
	cmd.FlagOrderPath(&orderPath)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagPlanPath(&planPath)
	cmd.FlagVersion(&printVersion)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()

	if printVersion {
		cmd.ExitWithVersion()
	}
	cmd.Exit(detect())
}

func detect() error {
	order, err := compat.ReadOrder(orderPath, buildpacksDir)
	if err != nil {
		return cmd.FailErr(err, "read legacy buildpack order file")
	}

	if len(order) == 0 {
		order, err = lifecycle.ReadOrder(orderPath)
		if err != nil {
			return cmd.FailErr(err, "read buildpack order file")
		}
	}

	group, plan, err := order.Detect(&lifecycle.DetectConfig{
		AppDir:        appDir,
		PlatformDir:   platformDir,
		BuildpacksDir: buildpacksDir,
		Out:           log.New(os.Stdout, "", 0),
	})
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedDetect, "detect")
	}

	if err := lifecycle.WriteTOML(groupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if err := lifecycle.WriteTOML(planPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}

	return nil
}
