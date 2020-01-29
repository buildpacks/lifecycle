package main

import (
	"errors"
	"flag"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

type restoreCmd struct {
	cacheDir      string
	cacheImageTag string
	groupPath     string
	layersDir     string
	uid           int
	gid           int
}

func (r *restoreCmd) Flags() {
	cmd.FlagCacheDir(&r.cacheDir)
	cmd.FlagCacheImage(&r.cacheImageTag)
	cmd.FlagGroupPath(&r.groupPath)
	cmd.FlagLayersDir(&r.layersDir)
	cmd.FlagUID(&r.uid)
	cmd.FlagGID(&r.gid)
	flag.Parse()

	if r.cacheImageTag == "" && r.cacheDir == "" {
		cmd.Logger.Warn("Not restoring cached layer data, no cache flag specified.")
	}
}

func (r *restoreCmd) Args() error {
	if flag.NArg() > 0 {
		return cmd.FailErrCode(errors.New("received unexpected Args"), cmd.CodeInvalidArgs, "parse arguments")
	}
	return nil
}

func (r *restoreCmd) Exec() error {
	group, err := lifecycle.ReadGroup(r.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}

	restorer := &lifecycle.Restorer{
		LayersDir:  r.layersDir,
		Buildpacks: group.Group,
		Logger:     cmd.Logger,
		UID:        r.uid,
		GID:        r.gid,
	}

	cacheStore, err := initCache(r.cacheImageTag, r.cacheDir)
	if err != nil {
		return err
	}

	if err := restorer.Restore(cacheStore); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "restore")
	}
	return nil
}
