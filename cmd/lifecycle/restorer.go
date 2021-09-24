package main

import (
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/common"
	"github.com/buildpacks/lifecycle/priv"
)

type restoreCmd struct {
	// flags: inputs
	analyzedPath  string
	cacheDir      string
	cacheImageTag string
	groupPath     string
	uid, gid      int

	restoreArgs
}

type restoreArgs struct {
	layersDir  string
	platform   Platform
	skipLayers bool

	// construct if necessary before dropping privileges
	keychain authn.Keychain
}

func (r *restoreCmd) DefineFlags() {
	cmd.FlagCacheDir(&r.cacheDir)
	cmd.FlagCacheImage(&r.cacheImageTag)
	cmd.FlagGroupPath(&r.groupPath)
	cmd.FlagLayersDir(&r.layersDir)
	cmd.FlagUID(&r.uid)
	cmd.FlagGID(&r.gid)
	if r.restoresLayerMetadata() {
		cmd.FlagAnalyzedPath(&r.analyzedPath)
		cmd.FlagSkipLayers(&r.skipLayers)
	}
}

func (r *restoreCmd) Args(nargs int, args []string) error {
	if nargs > 0 {
		return cmd.FailErrCode(errors.New("received unexpected Args"), cmd.CodeInvalidArgs, "parse arguments")
	}
	if r.cacheImageTag == "" && r.cacheDir == "" {
		cmd.DefaultLogger.Warn("Not restoring cached layer data, no cache flag specified.")
	}

	if r.groupPath == cmd.PlaceholderGroupPath {
		r.groupPath = cmd.DefaultGroupPath(r.platform.API(), r.layersDir)
	}

	if r.analyzedPath == cmd.PlaceholderAnalyzedPath {
		r.analyzedPath = cmd.DefaultAnalyzedPath(r.platform.API(), r.layersDir)
	}

	return nil
}

func (r *restoreCmd) Privileges() error {
	var err error
	r.keychain, err = auth.DefaultKeychain(r.registryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}

	if err := priv.EnsureOwner(r.uid, r.gid, r.layersDir, r.cacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(r.uid, r.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", r.uid, r.gid))
	}
	return nil
}

func (r *restoreCmd) Exec() error {
	group, err := lifecycle.ReadGroup(r.groupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}
	if err := verifyBuildpackApis(group); err != nil {
		return err
	}
	cacheStore, err := initCache(r.cacheImageTag, r.cacheDir, r.keychain)
	if err != nil {
		return err
	}

	var appMeta platform.LayersMetadata
	if r.restoresLayerMetadata() {
		var analyzedMd platform.AnalyzedMetadata
		if _, err := toml.DecodeFile(r.analyzedPath, &analyzedMd); err == nil {
			appMeta = analyzedMd.Metadata
		}
	}

	return r.restore(appMeta, group, cacheStore)
}

func (r *restoreCmd) registryImages() []string {
	if r.cacheImageTag != "" {
		return []string{r.cacheImageTag}
	}
	return []string{}
}

func (r restoreArgs) restore(layerMetadata platform.LayersMetadata, group buildpack.Group, cacheStore lifecycle.Cache) error {
	restorer := &lifecycle.Restorer{
		LayersDir:             r.layersDir,
		Buildpacks:            group.Group,
		Logger:                cmd.DefaultLogger,
		Platform:              r.platform,
		LayerMetadataRestorer: lifecycle.NewLayerMetadataRestorer(cmd.DefaultLogger, r.layersDir, r.skipLayers),
		LayersMetadata:        layerMetadata,
	}

	if err := restorer.Restore(cacheStore); err != nil {
		return cmd.FailErrCode(err, r.platform.CodeFor(common.RestoreError), "restore")
	}
	return nil
}

func (r *restoreArgs) restoresLayerMetadata() bool {
	return api.MustParse(r.platform.API()).AtLeast("0.7")
}
