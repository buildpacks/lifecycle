package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	*platform.Platform

	keychain authn.Keychain // construct if necessary before dropping privileges
}

// DefineFlags defines the flags that are considered valid and reads their values (if provided).
func (r *restoreCmd) DefineFlags() {
	switch {
	case r.PlatformAPI.AtLeast("0.10"):
		cli.FlagBuildImage(&r.BuildImageRef)
		cli.FlagCacheDir(&r.CacheDir)
		cli.FlagCacheImage(&r.CacheImageRef)
		cli.FlagGroupPath(&r.GroupPath)
		cli.FlagLayersDir(&r.LayersDir)
		cli.FlagUID(&r.UID)
		cli.FlagGID(&r.GID)
		cli.FlagAnalyzedPath(&r.AnalyzedPath)
		cli.FlagSkipLayers(&r.SkipLayers)
	case r.PlatformAPI.AtLeast("0.7"):
		cli.FlagCacheDir(&r.CacheDir)
		cli.FlagCacheImage(&r.CacheImageRef)
		cli.FlagGroupPath(&r.GroupPath)
		cli.FlagLayersDir(&r.LayersDir)
		cli.FlagUID(&r.UID)
		cli.FlagGID(&r.GID)
		cli.FlagAnalyzedPath(&r.AnalyzedPath)
		cli.FlagSkipLayers(&r.SkipLayers)
	default:
		cli.FlagCacheDir(&r.CacheDir)
		cli.FlagCacheImage(&r.CacheImageRef)
		cli.FlagGroupPath(&r.GroupPath)
		cli.FlagLayersDir(&r.LayersDir)
		cli.FlagUID(&r.UID)
		cli.FlagGID(&r.GID)
	}
}

// Args validates arguments and flags, and fills in default values.
func (r *restoreCmd) Args(nargs int, _ []string) error {
	if nargs > 0 {
		return cmd.FailErrCode(errors.New("received unexpected Args"), cmd.CodeForInvalidArgs, "parse arguments")
	}
	if err := platform.ResolveInputs(platform.Restore, r.LifecycleInputs, cmd.DefaultLogger); err != nil {
		return cmd.FailErrCode(err, cmd.CodeForInvalidArgs, "resolve inputs")
	}
	return nil
}

func (r *restoreCmd) Privileges() error {
	var err error
	r.keychain, err = auth.DefaultKeychain(r.RegistryImages()...)
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}
	if err = priv.EnsureOwner(r.UID, r.GID, r.LayersDir, r.CacheDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err = priv.RunAs(r.UID, r.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", r.UID, r.GID))
	}
	return nil
}

func (r *restoreCmd) Exec() error {
	if r.supportsBuildImageExtension() {
		if err := r.pullBuilderImageIfNeeded(); err != nil {
			return cmd.FailErr(err, "read builder image")
		}
	}

	group, err := lifecycle.ReadGroup(r.GroupPath)
	if err != nil {
		return cmd.FailErr(err, "read buildpack group")
	}
	if err := verifyBuildpackApis(group); err != nil {
		return err
	}

	cacheStore, err := initCache(r.CacheImageRef, r.CacheDir, r.keychain)
	if err != nil {
		return err
	}

	var appMeta platform.LayersMetadata
	if r.restoresLayerMetadata() {
		amd, err := platform.ReadAnalyzed(r.AnalyzedPath, cmd.DefaultLogger)
		if err == nil {
			appMeta = amd.Metadata
		}
	}

	return r.restore(appMeta, group, cacheStore)
}

func (r *restoreCmd) supportsBuildImageExtension() bool {
	return r.PlatformAPI.AtLeast("0.10")
}

func (r *restoreCmd) pullBuilderImageIfNeeded() error {
	if r.BuildImageRef == "" {
		return nil
	}
	if _, err := os.Stat(kanikoDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read kaniko directory: %w", err)
		}
		return nil
	}
	ref, authr, err := auth.ReferenceForRepoName(r.keychain, r.BuildImageRef)
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
	analyzedMD, err := lifecycle.Config.ReadAnalyzed(r.AnalyzedPath)
	if err != nil {
		return fmt.Errorf("failed to read analyzed metadata: %w", err)
	}
	digestRef, err := name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), buildImageDigest), name.WeakValidation)
	if err != nil {
		return fmt.Errorf("failed to get digest reference: %w", err)
	}
	analyzedMD.BuildImage = &platform.ImageIdentifier{Reference: digestRef.String()}
	return encoding.WriteTOML(r.AnalyzedPath, analyzedMD)
}

func (r *restoreCmd) restoresLayerMetadata() bool {
	return r.PlatformAPI.AtLeast("0.7")
}

func (r *restoreCmd) restore(layerMetadata platform.LayersMetadata, group buildpack.Group, cacheStore lifecycle.Cache) error {
	restorer := &lifecycle.Restorer{
		LayersDir:             r.LayersDir,
		Buildpacks:            group.Group,
		Logger:                cmd.DefaultLogger,
		PlatformAPI:           r.PlatformAPI,
		LayerMetadataRestorer: layer.NewDefaultMetadataRestorer(r.LayersDir, r.SkipLayers, cmd.DefaultLogger),
		LayersMetadata:        layerMetadata,
		SBOMRestorer: layer.NewSBOMRestorer(layer.SBOMRestorerOpts{
			LayersDir: r.LayersDir,
			Logger:    cmd.DefaultLogger,
			Nop:       r.SkipLayers,
		}, r.PlatformAPI),
	}
	if err := restorer.Restore(cacheStore); err != nil {
		return cmd.FailErrCode(err, r.CodeFor(platform.RestoreError), "restore")
	}
	return nil
}
