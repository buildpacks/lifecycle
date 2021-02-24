package main

import (
	"errors"
	"fmt"

	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/priv"
)

type restoreCmd struct {
	// flags: inputs
	cacheDir      string
	cacheImageTag string
	groupPath     string
	uid, gid      int
	restoreArgs
}

type restoreArgs struct {
	//inputs needed when run by creator
	imageName   string
	layersDir   string
	platformAPI string
	skipLayers  bool
	useDaemon   bool

	//construct if necessary before dropping privileges
	docker   client.CommonAPIClient
	keychain authn.Keychain
}

func (r *restoreCmd) DefineFlags() {
	cmd.FlagCacheDir(&r.cacheDir)
	cmd.FlagCacheImage(&r.cacheImageTag)
	cmd.FlagGroupPath(&r.groupPath)
	cmd.FlagLayersDir(&r.layersDir)
	cmd.FlagUID(&r.uid)
	cmd.FlagGID(&r.gid)
	if r.analyzeLayers() {
		cmd.FlagUseDaemon(&r.useDaemon)
		cmd.FlagSkipLayers(&r.skipLayers)
	}
}

func (r *restoreCmd) Args(nargs int, args []string) error {
	if !r.analyzeLayers() {
		if nargs > 0 {
			return cmd.FailErrCode(errors.New("received unexpected Args"), cmd.CodeInvalidArgs, "parse arguments")
		}
	} else {
		if nargs != 1 {
			return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeInvalidArgs, "parse arguments")
		}
		if args[0] == "" {
			return cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
		}
		r.imageName = args[0]
	}

	if r.cacheImageTag == "" && r.cacheDir == "" {
		cmd.DefaultLogger.Warn("Not restoring cached layer data, no cache flag specified.")
	}

	if r.groupPath == cmd.PlaceholderGroupPath {
		r.groupPath = cmd.DefaultGroupPath(r.platformAPI, r.layersDir)
	}

	return nil
}

func (r *restoreCmd) Privileges() error {
	var err error
	r.keychain, err = auth.DefaultKeychain(r.registryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}

	if r.analyzeLayers() {
		if r.useDaemon {
			var err error
			r.docker, err = priv.DockerClient()
			if err != nil {
				return cmd.FailErr(err, "initialize docker client")
			}
		}
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
	return r.restore(group, cacheStore)
}

func (r *restoreCmd) registryImages() []string {
	if r.cacheImageTag != "" {
		return []string{r.cacheImageTag}
	}
	return []string{}
}

func (r restoreArgs) restore(group buildpack.Group, cacheStore lifecycle.Cache) error {
	var (
		img imgutil.Image
		err error
	)

	if r.analyzeLayers() {
		if r.useDaemon {
			img, err = local.NewImage(
				r.imageName,
				r.docker,
				local.FromBaseImage(r.imageName),
			)
		} else {
			img, err = remote.NewImage(
				r.imageName,
				r.keychain,
				remote.FromBaseImage(r.imageName),
			)
		}

		if err != nil {
			return cmd.FailErr(err, "get previous image")
		}
	}

	restorer := &lifecycle.Restorer{
		LayersDir:   r.layersDir,
		Buildpacks:  group.Group,
		Logger:      cmd.DefaultLogger,
		PlatformAPI: api.MustParse(r.platformAPI),
	}

	if err := restorer.Restore(img, cacheStore); err != nil {
		return cmd.FailErrCode(err, cmd.CodeRestoreError, "restore")
	}
	return nil
}

func (r *restoreArgs) analyzeLayers() bool {
	return api.MustParse(r.platformAPI).Compare(api.MustParse("0.6")) >= 0
}
