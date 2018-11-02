package main

import (
	"flag"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"io/ioutil"
	"log"
	"os"
)

var (
	buildpacksDir string
	appDir        string
	orderPath     string
	groupPath     string
	planPath      string
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagOrderPath(&orderPath)

	cmd.FlagGroupPath(&groupPath)
	cmd.FlagPlanPath(&planPath)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments"))
	}
	cmd.Exit(detect())
}

func detect() error {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	buildpacks, err := lifecycle.NewBuildpackMap(buildpacksDir)
	if err != nil {
		return cmd.FailErr(err, "read buildpack directory")
	}
	order, err := buildpacks.ReadOrder(orderPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack order file")
	}

	info, group := order.Detect(logger, appDir)
	if group == nil {
		return cmd.FailCode(cmd.CodeFailedDetect, "detect")
	}

	if err := group.Write(groupPath); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if err := ioutil.WriteFile(planPath, info, 0666); err != nil {
		return cmd.FailErr(err, "write detect info")
	}

	return nil
}
