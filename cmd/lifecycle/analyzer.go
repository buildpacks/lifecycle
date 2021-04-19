package main

import (
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

type analyzeCmd struct {
	//flags: inputs
	analyzeArgs
	uid, gid int

	cacheDir      string // Platform API < 0.7
	cacheImageTag string // Platform API < 0.7
	groupPath     string // Platform API < 0.7

	//flags: paths to write data
	analyzedPath string
}

type analyzeArgs struct {
	imageName   string
	layersDir   string
	orderPath   string //nolint - Platform API >= 0.7
	runImageRef string //nolint - Platform API >= 0.7
	stackPath   string //nolint - Platform API >= 0.7
	skipLayers  bool   // Platform API < 0.7
	useDaemon   bool

	additionalTags cmd.StringSlice        //nolint Platform API >= 0.7
	cache          lifecycle.Cache        // Platform API < 0.7
	docker         client.CommonAPIClient //construct if necessary before dropping privileges
	group          buildpack.Group        // Platform API < 0.7
	keychain       authn.Keychain
	platform       cmd.Platform
}

func (a *analyzeCmd) DefineFlags() {
	cmd.FlagAnalyzedPath(&a.analyzedPath)
	if a.restoresLayerMetadata() {
		cmd.FlagCacheImage(&a.cacheImageTag)
		cmd.FlagCacheDir(&a.cacheDir)
		cmd.FlagGroupPath(&a.groupPath)
		cmd.FlagSkipLayers(&a.skipLayers)
	}
	cmd.FlagLayersDir(&a.layersDir)
	if a.platformAPIVersionGreaterThan06() {
		cmd.FlagOrderPath(&a.orderPath)
		cmd.FlagPreviousImage(&a.imageName)
		cmd.FlagRunImage(&a.runImageRef)
		cmd.FlagStackPath(&a.stackPath)
		cmd.FlagTags(&a.additionalTags)
	}
	cmd.FlagUseDaemon(&a.useDaemon)
	cmd.FlagUID(&a.uid)
	cmd.FlagGID(&a.gid)
}

func (a *analyzeCmd) Args(nargs int, args []string) error {
	if !a.platformAPIVersionGreaterThan06() {
		if nargs != 1 {
			return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeInvalidArgs, "parse arguments")
		}
		if args[0] == "" {
			return cmd.FailErrCode(errors.New("image argument is required"), cmd.CodeInvalidArgs, "parse arguments")
		}
		a.imageName = args[0]
	}

	if a.restoresLayerMetadata() {
		if a.cacheImageTag == "" && a.cacheDir == "" {
			cmd.DefaultLogger.Warn("Not restoring cached layer metadata, no cache flag specified.")
		}
	}

	if a.analyzedPath == cmd.PlaceholderAnalyzedPath {
		a.analyzedPath = cmd.DefaultAnalyzedPath(a.platform.API(), a.layersDir)
	}

	if a.groupPath == cmd.PlaceholderGroupPath {
		a.groupPath = cmd.DefaultGroupPath(a.platform.API(), a.layersDir)
	}

	if a.orderPath == cmd.PlaceholderOrderPath {
		a.orderPath = cmd.DefaultOrderPath(a.platform.API(), a.layersDir)
	}

	return nil
}

func (a *analyzeCmd) Privileges() error {
	var err error
	a.keychain, err = auth.DefaultKeychain(a.registryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}

	if a.useDaemon {
		var err error
		a.docker, err = priv.DockerClient()
		if err != nil {
			return cmd.FailErr(err, "initialize docker client")
		}
	}
	if err := priv.EnsureOwner(a.uid, a.gid, a.layersDir, a.cacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err := priv.RunAs(a.uid, a.gid); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", a.uid, a.gid))
	}
	return nil
}

func (a *analyzeCmd) Exec() error {
	var (
		group      buildpack.Group
		err        error
		cacheStore lifecycle.Cache
	)
	if a.restoresLayerMetadata() {
		group, err = lifecycle.ReadGroup(a.groupPath)
		if err != nil {
			return cmd.FailErr(err, "read buildpack group")
		}
		if err := verifyBuildpackApis(group); err != nil {
			return err
		}
		cacheStore, err = initCache(a.cacheImageTag, a.cacheDir, a.keychain)
		if err != nil {
			return cmd.FailErr(err, "initialize cache")
		}
		a.group = group
		a.cache = cacheStore
	}

	if a.orderPath != "" {
		_, err := lifecycle.ReadOrder(a.orderPath)
		if err != nil {
			return cmd.FailErr(err, "read buildpack order file")
		}
	}

	analyzedMD, err := a.analyze()
	if err != nil {
		return err
	}

	if err := lifecycle.WriteTOML(a.analyzedPath, analyzedMD); err != nil {
		return errors.Wrap(err, "write analyzed.toml")
	}

	return nil
}

func (aa analyzeArgs) analyze() (platform.AnalyzedMetadata, error) {
	var (
		img imgutil.Image
		err error
	)
	if aa.imageName != "" {
		if aa.useDaemon {
			img, err = local.NewImage(
				aa.imageName,
				aa.docker,
				local.FromBaseImage(aa.imageName),
			)
		} else {
			img, err = remote.NewImage(
				aa.imageName,
				aa.keychain,
				remote.FromBaseImage(aa.imageName),
			)
		}
		if err != nil {
			return platform.AnalyzedMetadata{}, cmd.FailErr(err, "get previous image")
		}
	}

	analyzedMD, err := (&lifecycle.Analyzer{
		Buildpacks:            aa.group.Group,
		Cache:                 aa.cache,
		Logger:                cmd.DefaultLogger,
		Platform:              aa.platform,
		Image:                 img,
		LayerMetadataRestorer: lifecycle.NewLayerMetadataRestorer(cmd.DefaultLogger, aa.layersDir, aa.platform, aa.skipLayers),
	}).Analyze()
	if err != nil {
		return platform.AnalyzedMetadata{}, cmd.FailErrCode(err, aa.platform.CodeFor(cmd.AnalyzeError), "analyzer")
	}
	return analyzedMD, nil
}

func (a *analyzeCmd) registryImages() []string {
	var registryImages []string
	if a.cacheImageTag != "" {
		registryImages = append(registryImages, a.cacheImageTag)
	}
	if !a.useDaemon {
		registryImages = append(registryImages, a.analyzeArgs.imageName)
	}
	return registryImages
}

func (a *analyzeCmd) restoresLayerMetadata() bool {
	return api.MustParse(a.platform.API()).Compare(api.MustParse("0.7")) < 0
}

func (a *analyzeCmd) platformAPIVersionGreaterThan06() bool {
	return api.MustParse(a.platform.API()).Compare(api.MustParse("0.7")) >= 0
}
