package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

var (
	buildpacksDir string
	appDir        string
	platformDir   string
	orderPath     string
	groupPath     string
	planPath      string
	printVersion  bool
	logLevel      string
)

func init() {
	cmd.FlagBuildpacksDir(&buildpacksDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagPlatformDir(&platformDir)
	cmd.FlagOrderPath(&orderPath)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagPlanPath(&planPath)
	cmd.FlagVersion(&printVersion)
	cmd.FlagLogLevel(&logLevel)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()

	if printVersion {
		cmd.ExitWithVersion()
	}

	if err := cmd.SetLogLevel(logLevel); err != nil {
		cmd.Exit(err)
	}

	cmd.Exit(detect())
}

func detect() error {
	order, err := lifecycle.ReadOrder(orderPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack order file")
	}

	env := &lifecycle.Env{
		LookupEnv: os.LookupEnv,
		Getenv:    os.Getenv,
		Setenv:    os.Setenv,
		Unsetenv:  os.Unsetenv,
		Environ:   os.Environ,
		Map:       lifecycle.POSIXBuildEnv,
	}
	fullEnv, err := env.WithPlatform(platformDir)
	if err != nil {
		return cmd.FailErr(err, "read full env")
	}
	group, plan, err := order.Detect(&lifecycle.DetectConfig{
		FullEnv:       fullEnv,
		ClearEnv:      env.List(),
		AppDir:        appDir,
		PlatformDir:   platformDir,
		BuildpacksDir: buildpacksDir,
		Logger:        cmd.Logger,
	})
	if err != nil {
		if err == lifecycle.ErrFail {
			cmd.Logger.Error("No buildpack groups passed detection.")
			cmd.Logger.Error("Please check that you are running against the correct path.")
		}
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
