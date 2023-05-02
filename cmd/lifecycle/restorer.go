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
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/cli"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	"github.com/buildpacks/lifecycle/platform/images"
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
	var (
		analyzedMD files.Analyzed
		err        error
	)
	if analyzedMD, err = files.ReadAnalyzed(r.AnalyzedPath, cmd.DefaultLogger); err == nil {
		if r.supportsBuildImageExtension() && r.BuildImageRef != "" {
			cmd.DefaultLogger.Debugf("Pulling builder image metadata...")
			buildImage, err := r.pullSparse(r.BuildImageRef)
			if err != nil {
				return cmd.FailErr(err, "read builder image")
			}
			digestRef, err := digestReference(r.BuildImageRef, buildImage)
			if err != nil {
				return cmd.FailErr(err, "get digest reference for builder image")
			}
			analyzedMD.BuildImage = &files.ImageIdentifier{Reference: digestRef}
		}
		if r.supportsRunImageExtension() && needsPulling(analyzedMD.RunImage) {
			cmd.DefaultLogger.Debugf("Pulling run image metadata...")
			runImageRef := analyzedMD.RunImageImage()
			if runImageRef == "" {
				runImageRef = analyzedMD.RunImage.Reference // older platforms don't populate Image
			}
			runImage, err := r.pullSparse(runImageRef)
			if err != nil {
				return cmd.FailErr(err, "read run image")
			}
			targetData, err := images.GetTargetMetadataFrom(runImage)
			if err != nil {
				return cmd.FailErr(err, "read target data from run image")
			}
			digestRef, err := digestReference(runImageRef, runImage)
			if err != nil {
				return cmd.FailErr(err, "get digest reference for builder image")
			}
			analyzedMD.RunImage = &files.RunImage{
				Reference:      digestRef,
				Image:          analyzedMD.RunImageImage(),
				Extend:         true,
				TargetMetadata: targetData,
			}
		} else if r.supportsTargetData() && needsUpdating(analyzedMD.RunImage) {
			cmd.DefaultLogger.Debugf("Updating analyzed metadata...")
			runImage, err := remote.NewImage(analyzedMD.RunImage.Reference, r.keychain)
			if err != nil {
				return cmd.FailErr(err, "read run image")
			}
			targetData, err := images.GetTargetMetadataFrom(runImage)
			if err != nil {
				return cmd.FailErr(err, "read target data from run image")
			}
			digestRef, err := digestReference(analyzedMD.RunImage.Reference, runImage)
			if err != nil {
				return cmd.FailErr(err, "get digest reference for builder image")
			}
			analyzedMD.RunImage = &files.RunImage{
				Reference:      digestRef,
				Image:          analyzedMD.RunImageImage(),
				Extend:         analyzedMD.RunImage.Extend,
				TargetMetadata: targetData,
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

func needsPulling(runImage *files.RunImage) bool {
	return runImage != nil && runImage.Extend
}

func needsUpdating(runImage *files.RunImage) bool {
	if runImage == nil {
		return false
	}
	if runImage.TargetMetadata != nil {
		return false
	}
	return true
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
	remoteImage, err := remote.NewV1Image(imageRef, r.keychain)
	if err != nil {
		return nil, fmt.Errorf("failed to get remote image: %w", err)
	}
	// check for usable kaniko dir
	if _, err := os.Stat(kanikoDir); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read kaniko directory: %w", err)
		}
		return nil, nil
	}
	// save to disk
	h, err := remoteImage.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote image digest: %w", err)
	}
	sparseImage, err := sparse.NewImage(
		filepath.Join(baseCacheDir, h.String()),
		remoteImage,
		layout.WithMediaTypes(imgutil.DefaultTypes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sparse image: %w", err)
	}
	if err = sparseImage.Save(); err != nil {
		return nil, fmt.Errorf("failed to save sparse image: %w", err)
	}
	return sparseImage, nil
}

func digestReference(imageRef string, image imgutil.Image) (string, error) {
	ir, err := name.ParseReference(imageRef)
	if err != nil {
		return "", err
	}
	_, err = name.NewDigest(ir.String())
	if err == nil {
		// if we already have a digest reference, return it
		return imageRef, nil
	}
	id, err := image.Identifier()
	if err != nil {
		return "", err
	}
	digest, err := name.NewDigest(id.String())
	if err != nil {
		return "", err
	}
	digestRef, err := name.NewDigest(fmt.Sprintf("%s@%s", ir.Context().Name(), digest.DigestStr()), name.WeakValidation)
	if err != nil {
		return "", err
	}
	return digestRef.String(), nil
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
