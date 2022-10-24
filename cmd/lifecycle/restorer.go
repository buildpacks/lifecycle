package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/internal/selective"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/priv"
)

const kanikoDir = "/kaniko"

type restoreCmd struct {
	// flags: inputs
	analyzedPath  string
	buildImageTag string
	cacheDir      string
	cacheImageTag string
	groupPath     string
	uid, gid      int

	restoreArgs
}

type restoreArgs struct {
	layersDir  string
	platform   *platform.Platform
	skipLayers bool

	// construct if necessary before dropping privileges
	keychain authn.Keychain
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (r *restoreCmd) DefineFlags() {
	switch {
	case r.platform.API().AtLeast("0.10"):
		cli.FlagBuildImage(&r.buildImageTag)
		cli.FlagCacheDir(&r.cacheDir)
		cli.FlagCacheImage(&r.cacheImageTag)
		cli.FlagGroupPath(&r.groupPath)
		cli.FlagLayersDir(&r.layersDir)
		cli.FlagUID(&r.uid)
		cli.FlagGID(&r.gid)
		cli.FlagAnalyzedPath(&r.analyzedPath)
		cli.FlagSkipLayers(&r.skipLayers)
	case r.platform.API().AtLeast("0.7"):
		cli.FlagCacheDir(&r.cacheDir)
		cli.FlagCacheImage(&r.cacheImageTag)
		cli.FlagGroupPath(&r.groupPath)
		cli.FlagLayersDir(&r.layersDir)
		cli.FlagUID(&r.uid)
		cli.FlagGID(&r.gid)
		cli.FlagAnalyzedPath(&r.analyzedPath)
		cli.FlagSkipLayers(&r.skipLayers)
	default:
		cli.FlagCacheDir(&r.cacheDir)
		cli.FlagCacheImage(&r.cacheImageTag)
		cli.FlagGroupPath(&r.groupPath)
		cli.FlagLayersDir(&r.layersDir)
		cli.FlagUID(&r.uid)
		cli.FlagGID(&r.gid)
	}
}

// Args validates arguments and flags, and fills in default values.
func (r *restoreCmd) Args(nargs int, args []string) error {
	if nargs > 0 {
		return cmd.FailErrCode(errors.New("received unexpected Args"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	if r.cacheImageTag == "" && r.cacheDir == "" {
		cmd.DefaultLogger.Warn("Not restoring cached layer data, no cache flag specified.")
	}

	if r.groupPath == platform.PlaceholderGroupPath {
		r.groupPath = cli.DefaultGroupPath(r.platform.API().String(), r.layersDir)
	}

	if r.analyzedPath == platform.PlaceholderAnalyzedPath {
		r.analyzedPath = cli.DefaultAnalyzedPath(r.platform.API().String(), r.layersDir)
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

func (r *restoreCmd) registryImages() []string {
	var images []string
	images = appendNotEmpty(images, r.cacheImageTag)
	images = appendNotEmpty(images, r.buildImageTag)
	return images
}

func (r *restoreCmd) Exec() error {
	if r.supportsBuildImageExtension() {
		if err := r.pullBuilderImageIfNeeded(); err != nil {
			return cmd.FailErr(err, "read builder image")
		}
	}

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
		var analyzedMD platform.AnalyzedMetadata
		if _, err := toml.DecodeFile(r.analyzedPath, &analyzedMD); err == nil {
			appMeta = analyzedMD.Metadata
		}
	}

	return r.restore(appMeta, group, cacheStore)
}

func (r *restoreArgs) supportsBuildImageExtension() bool {
	return r.platform.API().AtLeast("0.10")
}

func (r *restoreCmd) pullBuilderImageIfNeeded() error {
	if r.buildImageTag == "" {
		return nil
	}
	if _, err := os.Stat(kanikoDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read kaniko directory: %w", err)
		}
		return nil
	}
	ref, authr, err := auth.ReferenceForRepoName(r.keychain, r.buildImageTag)
	if err != nil {
		return fmt.Errorf("failed to get reference: %w", err)
	}
	remoteImage, err := remote.Image(ref, remote.WithAuth(authr))
	if err != nil {
		return fmt.Errorf("failed to read image: %w", err)
	}
	buildImageHash, err := remoteImage.Digest()
	if err != nil {
		return fmt.Errorf("failed to get digest: %w", err)
	}
	buildImageDigest := buildImageHash.String()
	baseCacheDir := filepath.Join(kanikoDir, "cache", "base")
	if err = os.MkdirAll(baseCacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory")
	}
	layoutPath, err := selective.Write(filepath.Join(baseCacheDir, buildImageDigest), empty.Index)
	if err != nil {
		return fmt.Errorf("failed to write layout path: %w", err)
	}
	if err = layoutPath.AppendImage(remoteImage); err != nil {
		return fmt.Errorf("failed to append image: %w", err)
	}
	// record digest in analyzed.toml
	analyzedMD, err := lifecycle.Config.ReadAnalyzed(r.analyzedPath)
	if err != nil {
		return fmt.Errorf("failed to read analyzed metadata: %w", err)
	}
	digestRef, err := name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), buildImageDigest), name.WeakValidation)
	if err != nil {
		return fmt.Errorf("failed to get digest reference: %w", err)
	}
	analyzedMD.BuildImage = &platform.ImageIdentifier{Reference: digestRef.String()}
	return encoding.WriteTOML(r.analyzedPath, analyzedMD)
}

func (r *restoreArgs) restoresLayerMetadata() bool {
	return r.platform.API().AtLeast("0.7")
}

func (r restoreArgs) restore(layerMetadata platform.LayersMetadata, group buildpack.Group, cacheStore lifecycle.Cache) error {
	restorer := &lifecycle.Restorer{
		LayersDir:             r.layersDir,
		Buildpacks:            group.Group,
		Logger:                cmd.DefaultLogger,
		Platform:              r.platform,
		LayerMetadataRestorer: layer.NewDefaultMetadataRestorer(r.layersDir, r.skipLayers, cmd.DefaultLogger),
		LayersMetadata:        layerMetadata,
		SBOMRestorer: layer.NewSBOMRestorer(layer.SBOMRestorerOpts{
			LayersDir: r.layersDir,
			Logger:    cmd.DefaultLogger,
			Nop:       r.skipLayers,
		}, r.platform.API()),
	}

	if err := restorer.Restore(cacheStore); err != nil {
		return cmd.FailErrCode(err, r.platform.CodeFor(platform.RestoreError), "restore")
	}
	return nil
}
