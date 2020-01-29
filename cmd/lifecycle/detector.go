package main

import (
	"flag"
	"os"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

type detectCmd struct {
	buildpacksDir string
	appDir        string
	platformDir   string
	orderPath     string
	groupPath     string
	planPath      string
}

func (d *detectCmd) Flags() {
	cmd.FlagBuildpacksDir(&d.buildpacksDir)
	cmd.FlagAppDir(&d.appDir)
	cmd.FlagPlatformDir(&d.platformDir)
	cmd.FlagOrderPath(&d.orderPath)
	cmd.FlagGroupPath(&d.groupPath)
	cmd.FlagPlanPath(&d.planPath)
	flag.Parse()
}

func (d *detectCmd) Args() error {
	return nil
}

func (d *detectCmd) Exec() error {
	order, err := lifecycle.ReadOrder(d.orderPath)
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
	fullEnv, err := env.WithPlatform(d.platformDir)
	if err != nil {
		return cmd.FailErr(err, "read full env")
	}
	group, plan, err := order.Detect(&lifecycle.DetectConfig{
		FullEnv:       fullEnv,
		ClearEnv:      env.List(),
		AppDir:        d.appDir,
		PlatformDir:   d.platformDir,
		BuildpacksDir: d.buildpacksDir,
		Logger:        cmd.Logger,
	})
	if err != nil {
		if err == lifecycle.ErrFail {
			cmd.Logger.Error("No buildpack groups passed detection.")
			cmd.Logger.Error("Please check that you are running against the correct path.")
		}
		return cmd.FailErrCode(err, cmd.CodeFailedDetect, "detect")
	}

	if err := lifecycle.WriteTOML(d.groupPath, group); err != nil {
		return cmd.FailErr(err, "write buildpack group")
	}

	if err := lifecycle.WriteTOML(d.planPath, plan); err != nil {
		return cmd.FailErr(err, "write detect plan")
	}
	return nil
}
