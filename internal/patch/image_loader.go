// Package patch provides functionality for patching buildpack-contributed layers
// in OCI images during the rebase phase of the Cloud Native Buildpacks lifecycle.
package patch

import (
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/moby/moby/client"

	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
)

// ImageLoader handles loading patch images from registries or the local Docker daemon.
type ImageLoader struct {
	Keychain           authn.Keychain
	InsecureRegistries []string
	Logger             log.Logger
	UseDaemon          bool
	DockerClient       client.APIClient
}

// NewImageLoader creates a new ImageLoader.
func NewImageLoader(keychain authn.Keychain, insecureRegistries []string, logger log.Logger, useDaemon bool, dockerClient client.APIClient) *ImageLoader {
	return &ImageLoader{
		Keychain:           keychain,
		InsecureRegistries: insecureRegistries,
		Logger:             logger,
		UseDaemon:          useDaemon,
		DockerClient:       dockerClient,
	}
}

// LoadPatchImage loads a patch image from the registry.
// It tries the primary image first, then falls back to mirrors if the primary fails.
// For multi-arch images, it selects the matching OS/arch variant.
// Returns nil without error if no matching variant is found (skip with warning).
func (l *ImageLoader) LoadPatchImage(patch files.LayerPatch, targetOS, targetArch, targetVariant string) (imgutil.Image, files.LayersMetadataCompat, error) {
	// Try primary image first
	img, md, err := l.tryLoadImage(patch.PatchImage, targetOS, targetArch, targetVariant)
	if err == nil && img != nil {
		return img, md, nil
	}

	primaryErr := err
	if primaryErr == nil {
		primaryErr = fmt.Errorf("no matching variant found")
	}

	// Try mirrors as fallback
	for _, mirror := range patch.PatchImageMirrors {
		l.Logger.Debugf("Primary patch image %s failed, trying mirror: %s", patch.PatchImage, mirror)
		img, md, err = l.tryLoadImage(mirror, targetOS, targetArch, targetVariant)
		if err == nil && img != nil {
			return img, md, nil
		}
	}

	// All attempts failed
	return nil, files.LayersMetadataCompat{}, fmt.Errorf("failed to load patch image %s (and %d mirrors): %w",
		patch.PatchImage, len(patch.PatchImageMirrors), primaryErr)
}

// tryLoadImage attempts to load a single image reference.
func (l *ImageLoader) tryLoadImage(imageRef, targetOS, targetArch, targetVariant string) (imgutil.Image, files.LayersMetadataCompat, error) {
	var img imgutil.Image
	var err error

	if l.UseDaemon {
		img, err = local.NewImage(
			imageRef,
			l.DockerClient,
			local.FromBaseImage(imageRef),
		)
	} else {
		opts := []imgutil.ImageOption{
			remote.FromBaseImage(imageRef),
		}
		opts = append(opts, image.GetInsecureOptions(l.InsecureRegistries)...)
		img, err = remote.NewImage(imageRef, l.Keychain, opts...)
	}
	if err != nil {
		return nil, files.LayersMetadataCompat{}, fmt.Errorf("failed to access patch image %s: %w", imageRef, err)
	}

	if !img.Found() {
		return nil, files.LayersMetadataCompat{}, fmt.Errorf("patch image %s not found", imageRef)
	}

	// Check OS/arch compatibility
	imgOS, err := img.OS()
	if err != nil {
		return nil, files.LayersMetadataCompat{}, fmt.Errorf("failed to get OS from patch image: %w", err)
	}

	imgArch, err := img.Architecture()
	if err != nil {
		return nil, files.LayersMetadataCompat{}, fmt.Errorf("failed to get architecture from patch image: %w", err)
	}

	imgVariant, _ := img.Variant() // Variant may not be set

	// Check if OS/arch matches
	if imgOS != targetOS || imgArch != targetArch {
		l.Logger.Warnf("Patch image %s has OS/arch %s/%s, but target is %s/%s; skipping",
			imageRef, imgOS, imgArch, targetOS, targetArch)
		return nil, files.LayersMetadataCompat{}, nil
	}

	// Check variant if both are specified
	if targetVariant != "" && imgVariant != "" && imgVariant != targetVariant {
		l.Logger.Warnf("Patch image %s has variant %s, but target is %s; skipping",
			imageRef, imgVariant, targetVariant)
		return nil, files.LayersMetadataCompat{}, nil
	}

	// Extract metadata from the patch image
	md, err := l.extractLayerMetadata(img)
	if err != nil {
		return nil, files.LayersMetadataCompat{}, fmt.Errorf("failed to extract metadata from patch image: %w", err)
	}

	return img, md, nil
}

// extractLayerMetadata extracts the lifecycle layers metadata from the image label.
func (l *ImageLoader) extractLayerMetadata(img imgutil.Image) (files.LayersMetadataCompat, error) {
	var md files.LayersMetadataCompat
	if err := image.DecodeLabel(img, platform.LifecycleMetadataLabel, &md); err != nil {
		return files.LayersMetadataCompat{}, fmt.Errorf("failed to decode lifecycle metadata label: %w", err)
	}
	return md, nil
}
