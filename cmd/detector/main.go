package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
)

var (
	buildpacksDir string
	appDir        string
	platformDir   string
	orderPath     string

	groupPath string
	planPath  string
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagPlatformDir(&platformDir)
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
	errLog := log.New(os.Stderr, "", log.LstdFlags)
	outLog := log.New(os.Stdout, "", log.LstdFlags)

	buildpacks, err := lifecycle.NewBuildpackMap(buildpacksDir)
	if err != nil {
		return cmd.FailErr(err, "read buildpack directory")
	}
	order, err := buildpacks.ReadOrder(orderPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack order file")
	}

	info, group := order.Detect(&lifecycle.DetectConfig{
		AppDir:      appDir,
		PlatformDir: platformDir,
		Out:         outLog,
		Err:         errLog,
	})
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
