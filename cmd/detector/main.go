package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
)

var (
	buildpacksDir string
	orderPath     string
	groupPath     string
	planPath      string
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagOrderPath(&orderPath)

	cmd.FlagGroupPath(&groupPath)
	cmd.FlagPlanPath(&planPath)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || buildpacksDir == "" || orderPath == "" || groupPath == "" || planPath == "" {
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

	info, group := order.Detect(logger, filepath.Join(lifecycle.DefaultLaunchDir, "app"))
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
