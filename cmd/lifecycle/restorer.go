package main

import (
	"errors"
	"fmt"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/priv"
)

type restoreCmd struct {
	// flags: inputs
	cacheDir            string
	cacheImageTag       string
	groupPath           string
	privilegedGroupPath string
	layersDir           string
	uid, gid            int
}

func (r *restoreCmd) Init() {
	cmd.FlagCacheDir(&r.cacheDir)
	cmd.FlagCacheImage(&r.cacheImageTag)
	cmd.FlagGroupPath(&r.groupPath)
	cmd.FlagLayersDir(&r.layersDir)
	cmd.FlagUID(&r.uid)
	cmd.FlagGID(&r.gid)
	cmd.FlagPrivilegedGroupPath(&r.privilegedGroupPath)
}

func (r *restoreCmd) Args(nargs int, args []string) error {
	if nargs > 0 {
		return cmd.FailErrCode(errors.New("received unexpected Args"), cmd.CodeInvalidArgs, "parse arguments")
	}
	if r.cacheImageTag == "" && r.cacheDir == "" {
		cmd.DefaultLogger.Warn("Not restoring cached layer data, no cache flag specified.")
	}
	return nil
}

func (r *restoreCmd) Privileges() error {
	if err := priv.EnsureOwner(r.uid, r.gid, r.layersDir, r.cacheDir); err != nil {
		cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(r.uid, r.gid); err != nil {
		cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", r.uid, r.gid))
	}
	return nil
}

func (r *restoreCmd) Exec() error {
	group, privGroup, err := lifecycle.ReadGroups(r.groupPath, r.privilegedGroupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}
	if err := verifyBuildpackApis(group, privGroup); err != nil {
		return err
	}
	cacheStore, err := initCache(r.cacheImageTag, r.cacheDir)
	if err != nil {
		return err
	}
	return restore(r.layersDir, group, privGroup, cacheStore)
}

func restore(layersDir string, group, privGroup lifecycle.BuildpackGroup, cacheStore lifecycle.Cache) error {
	restorer := &lifecycle.Restorer{
		LayersDir:  layersDir,
		Buildpacks: append(privGroup.Group, group.Group...),
		Logger:     cmd.DefaultLogger,
	}

	if err := restorer.Restore(cacheStore); err != nil {
		return cmd.FailErrCode(err, cmd.CodeRestoreError, "restore")
	}
	return nil
}
