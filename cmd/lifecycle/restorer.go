package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
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
	if r.PlatformAPI.AtLeast("0.12") {
		cli.FlagGeneratedDir(&r.GeneratedDir)
	}
	if r.PlatformAPI.AtLeast("0.10") {
		cli.FlagBuildImage(&r.BuildImageRef)
	}
	if r.PlatformAPI.AtLeast("0.7") {
		cli.FlagAnalyzedPath(&r.AnalyzedPath)
		cli.FlagSkipLayers(&r.SkipLayers)
	}
	cli.FlagCacheDir(&r.CacheDir)
	cli.FlagCacheImage(&r.CacheImageRef)
	cli.FlagGroupPath(&r.GroupPath)
	cli.FlagUID(&r.UID)
	cli.FlagGID(&r.GID)
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
	var analyzedMD platform.AnalyzedMetadata
	if _, err := toml.DecodeFile(r.AnalyzedPath, &analyzedMD); err == nil {
		if r.supportsBuildImageExtension() {
			cmd.DefaultLogger.Debugf("Pulling builder image metadata...")
			_, digest, err := r.pullSparse(r.BuildImageRef)
			if err != nil {
				return cmd.FailErr(err, "read builder image")
			}
			analyzedMD.BuildImage = &platform.ImageIdentifier{Reference: digest.String()}
		}
		if r.supportsRunImageExtension() && needsPulling(analyzedMD.RunImage) {
			cmd.DefaultLogger.Debugf("Pulling run image metadata...")
			runImage, digest, err := r.pullSparse(analyzedMD.RunImage.Reference)
			if err != nil {
				return cmd.FailErr(err, "reading run image")
			}
			targetData, err := platform.ReadTargetData(runImage)
			if err != nil {
				return cmd.FailErr(err, "reading target data from run image")
			}
			analyzedMD.RunImage = &platform.RunImage{
				Reference:      digest.String(),
				Extend:         true,
				TargetMetadata: &targetData,
			}
		} else if needsUpdating(analyzedMD.RunImage) {
			cmd.DefaultLogger.Debugf("Updating analyzed metadata...")
			runImage, digest, err := newRemoteImage(analyzedMD.RunImage.Reference, r.keychain)
			if err != nil {
				return cmd.FailErr(err, "reading run image")
			}
			targetData, err := platform.ReadTargetData(runImage)
			if err != nil {
				return cmd.FailErr(err, "reading target data from run image")
			}
			analyzedMD.RunImage = &platform.RunImage{
				Reference:      digest.String(),
				Extend:         analyzedMD.RunImage.Extend,
				TargetMetadata: &targetData,
			}
		}
		if err = encoding.WriteTOML(r.AnalyzedPath, analyzedMD); err != nil {
			return cmd.FailErr(err, "write analyzed metadata")
		}
	} else {
		cmd.DefaultLogger.Warnf("Not using analyzed data, usable file not found: %s", err)
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
		appMeta = analyzedMD.Metadata
	}

	return r.restore(appMeta, group, cacheStore)
}

func needsPulling(runImage *platform.RunImage) bool {
	return runImage != nil && runImage.Extend
}

func needsUpdating(runImage *platform.RunImage) bool {
	if runImage == nil {
		return false
	}
	if runImage.TargetMetadata == nil {
		return true
	}
	digest, err := name.NewDigest(runImage.Reference)
	if err != nil {
		return true
	}
	return digest.DigestStr() == ""
}

func (r *restoreCmd) supportsBuildImageExtension() bool {
	return r.PlatformAPI.AtLeast("0.10")
}

func (r *restoreCmd) supportsRunImageExtension() bool {
	return r.PlatformAPI.AtLeast("0.12")
}

func (r *restoreCmd) pullSparse(imageRef string) (image v1.Image, digest name.Digest, err error) {
	if imageRef == "" {
		return nil, name.Digest{}, nil
	}
	baseCacheDir := filepath.Join(kanikoDir, "cache", "base")
	if err := os.MkdirAll(baseCacheDir, 0755); err != nil {
		return nil, name.Digest{}, fmt.Errorf("failed to create cache directory: %w", err)
	}
	// get remote image
	remoteImage, digest, err := newRemoteImage(imageRef, r.keychain)
	if err != nil {
		return nil, name.Digest{}, fmt.Errorf("failed to get remote image: %w", err)
	}
	// check for usable kaniko dir
	if _, err := os.Stat(kanikoDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, name.Digest{}, fmt.Errorf("failed to read kaniko directory: %w", err)
		}
		return nil, name.Digest{}, nil
	}
	// save to disk
	layoutPath, err := selective.Write(filepath.Join(baseCacheDir, digest.DigestStr()), empty.Index)
	if err != nil {
		return nil, name.Digest{}, fmt.Errorf("failed to write to layout path: %w", err)
	}
	if err = layoutPath.AppendImage(remoteImage); err != nil {
		return nil, name.Digest{}, fmt.Errorf("failed to append image: %w", err)
	}
	return remoteImage, digest, nil
}

func newRemoteImage(imageRef string, keychain authn.Keychain) (image v1.Image, digest name.Digest, err error) {
	ref, authr, authErr := auth.ReferenceForRepoName(keychain, imageRef)
	if authErr != nil {
		err = authErr
		return
	}
	if image, err = remote.Image(ref, remote.WithAuth(authr)); err != nil {
		return
	}
	var imageHash v1.Hash
	if imageHash, err = image.Digest(); err != nil {
		return
	}
	digest, err = name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), imageHash.String()), name.WeakValidation)
	return
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
