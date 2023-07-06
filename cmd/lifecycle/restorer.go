package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layout"
	"github.com/buildpacks/imgutil/layout/sparse"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
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
	cli.FlagGID(&r.GID)
	cli.FlagGroupPath(&r.GroupPath)
	cli.FlagLayersDir(&r.LayersDir)
	cli.FlagUID(&r.UID)
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
	if err = priv.EnsureOwner(r.UID, r.GID, r.LayersDir, r.CacheDir, r.KanikoDir); err != nil {
		return cmd.FailErr(err, "chown volumes")
	}
	if err = priv.RunAs(r.UID, r.GID); err != nil {
		return cmd.FailErr(err, fmt.Sprintf("exec as user %d:%d", r.UID, r.GID))
	}
	return nil
}

func (r *restoreCmd) Exec() error {
	var (
		analyzedMD files.Analyzed
		err        error
	)
	if analyzedMD, err = files.ReadAnalyzed(r.AnalyzedPath, cmd.DefaultLogger); err == nil {
		if r.supportsBuildImageExtension() && r.BuildImageRef != "" {
			cmd.DefaultLogger.Debugf("Pulling builder image metadata for %s...", r.BuildImageRef)
			remoteBuildImage, err := r.pullSparse(r.BuildImageRef)
			if err != nil {
				return cmd.FailErr(err, "pull builder image")
			}
			digestRef, err := remoteBuildImage.Identifier()
			if err != nil {
				return cmd.FailErr(err, "get digest reference for builder image")
			}
			analyzedMD.BuildImage = &files.ImageIdentifier{Reference: digestRef.String()}
			cmd.DefaultLogger.Debugf("Adding build image info to analyzed metadata: ")
			cmd.DefaultLogger.Debugf(encoding.ToJSONMaybe(analyzedMD.BuildImage))
		}
		var (
			remoteRunImage imgutil.Image
		)
		runImageName := analyzedMD.RunImageImage() // FIXME: if we have a digest reference available in `Reference` (e.g., in the non-daemon case) we should use it
		if r.supportsRunImageExtension() && needsPulling(analyzedMD.RunImage) {
			cmd.DefaultLogger.Debugf("Pulling run image metadata for %s...", runImageName)
			remoteRunImage, err = r.pullSparse(runImageName)
			if err != nil {
				return cmd.FailErr(err, "pull run image")
			}
			// update analyzed metadata, even if we only needed to pull the image metadata, because
			// the extender needs a digest reference in analyzed.toml,
			// and daemon images will only have a daemon image ID
			if err = updateAnalyzedMD(&analyzedMD, remoteRunImage); err != nil {
				return cmd.FailErr(err, "update analyzed metadata")
			}
		} else if r.supportsTargetData() && needsUpdating(analyzedMD.RunImage) {
			cmd.DefaultLogger.Debugf("Updating run image info in analyzed metadata...")
			remoteRunImage, err = remote.NewImage(runImageName, r.keychain)
			if err != nil || !remoteRunImage.Found() {
				return cmd.FailErr(err, "pull run image")
			}
			if err = updateAnalyzedMD(&analyzedMD, remoteRunImage); err != nil {
				return cmd.FailErr(err, "update analyzed metadata")
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

	var appMeta files.LayersMetadata
	if r.restoresLayerMetadata() {
		appMeta = analyzedMD.LayersMetadata
	}

	return r.restore(appMeta, group, cacheStore)
}

func updateAnalyzedMD(analyzedMD *files.Analyzed, remoteRunImage imgutil.Image) error {
	digestRef, err := remoteRunImage.Identifier()
	if err != nil {
		return cmd.FailErr(err, "get digest reference for run image")
	}
	targetData, err := platform.GetTargetMetadata(remoteRunImage)
	if err != nil {
		return cmd.FailErr(err, "read target data from run image")
	}
	cmd.DefaultLogger.Debugf("Run image info in analyzed metadata was: ")
	cmd.DefaultLogger.Debugf(encoding.ToJSONMaybe(analyzedMD.RunImage))
	analyzedMD.RunImage.Reference = digestRef.String()
	analyzedMD.RunImage.TargetMetadata = targetData
	cmd.DefaultLogger.Debugf("Run image info in analyzed metadata is: ")
	cmd.DefaultLogger.Debugf(encoding.ToJSONMaybe(analyzedMD.RunImage))
	return nil
}

func needsPulling(runImage *files.RunImage) bool {
	if runImage == nil {
		// sanity check to prevent panic, should be unreachable
		return false
	}
	return runImage.Extend
}

func needsUpdating(runImage *files.RunImage) bool {
	if runImage == nil {
		// sanity check to prevent panic, should be unreachable
		return false
	}
	if runImage.Reference != "" && isPopulated(runImage.TargetMetadata) {
		return false
	}
	return true
}

func isPopulated(metadata *files.TargetMetadata) bool {
	return metadata != nil && metadata.OS != ""
}

func (r *restoreCmd) supportsBuildImageExtension() bool {
	return r.PlatformAPI.AtLeast("0.10")
}

func (r *restoreCmd) supportsRunImageExtension() bool {
	return r.PlatformAPI.AtLeast("0.12")
}

func (r *restoreCmd) supportsTargetData() bool {
	return r.PlatformAPI.AtLeast("0.12")
}

func (r *restoreCmd) pullSparse(imageRef string) (imgutil.Image, error) {
	baseCacheDir := filepath.Join(kanikoDir, "cache", "base")
	if err := os.MkdirAll(baseCacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	// get remote image
	remoteImage, err := remote.NewImage(imageRef, r.keychain, remote.FromBaseImage(imageRef))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize remote image: %w", err)
	}
	if !remoteImage.Found() {
		return nil, fmt.Errorf("failed to get remote image")
	}
	// check for usable kaniko dir
	if _, err := os.Stat(kanikoDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read kaniko directory: %w", err)
		}
		return nil, nil
	}
	// save to disk
	h, err := remoteImage.UnderlyingImage().Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote image digest: %w", err)
	}
	path := filepath.Join(baseCacheDir, h.String())
	cmd.DefaultLogger.Debugf("Saving image metadata to %s...", path)
	sparseImage, err := sparse.NewImage(
		path,
		remoteImage.UnderlyingImage(),
		layout.WithMediaTypes(imgutil.DefaultTypes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sparse image: %w", err)
	}
	if err = sparseImage.Save(); err != nil {
		return nil, fmt.Errorf("failed to save sparse image: %w", err)
	}
	return remoteImage, nil
}

func (r *restoreCmd) restoresLayerMetadata() bool {
	return r.PlatformAPI.AtLeast("0.7")
}

func (r *restoreCmd) restore(layerMetadata files.LayersMetadata, group buildpack.Group, cacheStore lifecycle.Cache) error {
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
