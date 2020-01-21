package main

import (
	"errors"
	"flag"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

type restoreFlags struct {
	cacheDir      string
	cacheImageTag string
	groupPath     string
	layersDir     string
	uid           int
	gid           int
}

func parseRestoreFlags() (restoreFlags, error) {
	f := restoreFlags{}
	cmd.FlagCacheDir(&f.cacheDir)
	cmd.FlagCacheImage(&f.cacheImageTag)
	cmd.FlagGroupPath(&f.groupPath)
	cmd.FlagLayersDir(&f.layersDir)
	cmd.FlagUID(&f.uid)
	cmd.FlagGID(&f.gid)

	flag.Parse()
	commonFlags()

	if flag.NArg() > 0 {
		return restoreFlags{}, cmd.FailErrCode(errors.New("received unexpected args"), cmd.CodeInvalidArgs, "parse arguments")
	}
	if f.cacheImageTag == "" && f.cacheDir == "" {
		cmd.Logger.Warn("Not restoring cached layer data, no cache flag specified.")
	}
	return f, nil
}

func restore(f restoreFlags) error {
	group, err := lifecycle.ReadGroup(f.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	restorer := &lifecycle.Restorer{
		LayersDir:  f.layersDir,
		Buildpacks: group.Group,
		Logger:     cmd.Logger,
		UID:        f.uid,
		GID:        f.gid,
	}

	cacheStore, err := initCache(f.cacheImageTag, f.cacheDir)
	if err != nil {
		return err
	}

	if err := restorer.Restore(cacheStore); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "restore")
	}
	return nil
}
