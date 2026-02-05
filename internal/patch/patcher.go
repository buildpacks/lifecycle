package patch

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/moby/client"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/files"
)

// PatchResult represents the result of applying a single layer patch.
type PatchResult struct {
	BuildpackID   string
	BuildpackIdx  int
	LayerName     string
	OriginalSHA   string
	NewSHA        string
	NewLayerData  interface{}
	NewLayerFlags buildpack.LayerMetadataFile
}

// LayerPatcher handles the patching of buildpack-contributed layers.
type LayerPatcher struct {
	Logger             log.Logger
	Keychain           authn.Keychain
	InsecureRegistries []string
	UseDaemon          bool
	DockerClient       client.APIClient
}

// NewLayerPatcher creates a new LayerPatcher.
func NewLayerPatcher(logger log.Logger, keychain authn.Keychain, insecureRegistries []string, useDaemon bool, dockerClient client.APIClient) *LayerPatcher {
	return &LayerPatcher{
		Logger:             logger,
		Keychain:           keychain,
		InsecureRegistries: insecureRegistries,
		UseDaemon:          useDaemon,
		DockerClient:       dockerClient,
	}
}

// ApplyPatches applies all patches to the working image.
// This uses all-or-nothing semantics: all patches are validated first,
// and only applied if all validations pass.
// Returns the results, a cleanup function that MUST be called after the image is saved,
// and any error that occurred.
func (p *LayerPatcher) ApplyPatches(
	workingImage imgutil.Image,
	metadata *files.LayersMetadataCompat,
	patches files.LayerPatchesFile,
	targetOS, targetArch, targetVariant string,
) ([]PatchResult, func(), error) {
	noopCleanup := func() {}

	if len(patches.Patches) == 0 {
		return nil, noopCleanup, nil
	}

	p.Logger.Infof("Applying %d layer patch(es)", len(patches.Patches))

	// Create a temp directory for all layer patches
	tmpDir, err := os.MkdirTemp("", "layer-patches-")
	if err != nil {
		return nil, noopCleanup, fmt.Errorf("failed to create temp directory for patches: %w", err)
	}
	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	matcher := NewLayerMatcher()
	loader := NewPatchImageLoader(p.Keychain, p.InsecureRegistries, p.Logger, p.UseDaemon, p.DockerClient)

	// Phase 1: Validate all patches and collect the operations to perform
	var operations []patchOperation
	for i, patch := range patches.Patches {
		ops, err := p.validatePatch(patch, i, metadata, matcher, loader, targetOS, targetArch, targetVariant)
		if err != nil {
			cleanup()
			return nil, noopCleanup, fmt.Errorf("validation failed for patch %d (buildpack=%s, layer=%s): %w",
				i, patch.Buildpack, patch.Layer, err)
		}
		operations = append(operations, ops...)
	}

	if len(operations) == 0 {
		cleanup()
		p.Logger.Infof("No layers in this image matched any patch selectors")
		return nil, noopCleanup, nil
	}

	// Phase 2: Apply all validated operations
	var results []PatchResult
	for _, op := range operations {
		result, err := p.applyOperation(workingImage, metadata, op, tmpDir)
		if err != nil {
			cleanup()
			return nil, noopCleanup, fmt.Errorf("failed to apply patch for %s:%s: %w", op.match.BuildpackID, op.match.LayerName, err)
		}
		results = append(results, result)
	}

	p.Logger.Infof("Successfully applied %d layer patch(es)", len(results))
	return results, cleanup, nil
}

// patchOperation represents a validated patch operation ready to be applied.
type patchOperation struct {
	patchIndex    int
	match         MatchResult
	patchImage    imgutil.Image
	patchMetadata files.LayersMetadataCompat
	patchLayerMD  buildpack.LayerMetadata
}

// validatePatch validates a single patch and returns the operations to perform.
// Returns nil operations (not an error) if no layers match - this allows patch files
// to contain many potential patches that may or may not apply to a given app.
func (p *LayerPatcher) validatePatch(
	patch files.LayerPatch,
	patchIndex int,
	metadata *files.LayersMetadataCompat,
	matcher *LayerMatcher,
	loader *PatchImageLoader,
	targetOS, targetArch, targetVariant string,
) ([]patchOperation, error) {
	// Find matching layers in the working image
	matches := matcher.FindMatchingLayers(*metadata, patch)
	if len(matches) == 0 {
		// No matching layers - this is not an error, just skip this patch
		p.Logger.Debugf("Skipping patch %d: no matching layers for buildpack=%s, layer=%s",
			patchIndex, patch.Buildpack, patch.Layer)
		return nil, nil
	}

	// Load the patch image
	patchImage, patchMD, err := loader.LoadPatchImage(patch, targetOS, targetArch, targetVariant)
	if err != nil {
		return nil, fmt.Errorf("failed to load patch image: %w", err)
	}
	if patchImage == nil {
		// No matching variant found, skip
		p.Logger.Warnf("Skipping patch %d: no matching OS/arch variant in patch image", patchIndex)
		return nil, nil
	}

	var operations []patchOperation
	for _, match := range matches {
		// Find the corresponding layer in the patch image metadata
		patchLayerMD, err := findPatchLayer(patchMD, match.BuildpackID, match.LayerName)
		if err != nil {
			return nil, fmt.Errorf("patch image does not contain layer %s for buildpack %s: %w",
				match.LayerName, match.BuildpackID, err)
		}

		operations = append(operations, patchOperation{
			patchIndex:    patchIndex,
			match:         match,
			patchImage:    patchImage,
			patchMetadata: patchMD,
			patchLayerMD:  patchLayerMD,
		})

		p.Logger.Debugf("Validated patch for %s:%s (original SHA: %s, new SHA: %s)",
			match.BuildpackID, match.LayerName, match.LayerMetadata.SHA, patchLayerMD.SHA)
	}

	return operations, nil
}

// findPatchLayer finds a layer in the patch image metadata.
func findPatchLayer(patchMD files.LayersMetadataCompat, buildpackID, layerName string) (buildpack.LayerMetadata, error) {
	for _, bp := range patchMD.Buildpacks {
		if bp.ID == buildpackID {
			if layerMD, ok := bp.Layers[layerName]; ok {
				return layerMD, nil
			}
		}
	}
	return buildpack.LayerMetadata{}, fmt.Errorf("layer not found")
}

// applyOperation applies a single validated patch operation.
// The tmpDir parameter is a directory where temp files will be stored; these files
// must persist until the image is saved, so cleanup is handled by the caller.
func (p *LayerPatcher) applyOperation(
	workingImage imgutil.Image,
	metadata *files.LayersMetadataCompat,
	op patchOperation,
	tmpDir string,
) (PatchResult, error) {
	// Get the layer from the patch image
	// Note: imgutil doesn't support removing individual layers, so the old layer blob
	// remains but metadata will point to the new layer
	diffID := op.patchLayerMD.SHA
	if diffID == "" {
		return PatchResult{}, fmt.Errorf("patch layer has no SHA")
	}

	// Get the layer reader from the patch image
	layerReader, err := op.patchImage.GetLayer(diffID)
	if err != nil {
		return PatchResult{}, fmt.Errorf("failed to get layer %s from patch image: %w", diffID, err)
	}
	defer layerReader.Close()

	// Write the layer to a temporary file since imgutil requires a file path
	// Files are stored in tmpDir and cleaned up by the caller after the image is saved
	tmpFile, err := os.CreateTemp(tmpDir, "layer-patch-*.tar")
	if err != nil {
		return PatchResult{}, fmt.Errorf("failed to create temp file for layer: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = io.Copy(tmpFile, layerReader)
	tmpFile.Close()
	if err != nil {
		return PatchResult{}, fmt.Errorf("failed to write layer to temp file: %w", err)
	}

	// Make the path absolute
	absPath, err := filepath.Abs(tmpPath)
	if err != nil {
		return PatchResult{}, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Add the new layer to the working image
	// Using AddLayerWithDiffIDAndHistory to preserve the layer's identity
	err = workingImage.AddLayerWithDiffIDAndHistory(absPath, diffID, v1.History{})
	if err != nil {
		return PatchResult{}, fmt.Errorf("failed to add layer to working image: %w", err)
	}

	// Update the metadata for this layer
	bpIdx := op.match.BuildpackIndex
	layerName := op.match.LayerName
	originalSHA := metadata.Buildpacks[bpIdx].Layers[layerName].SHA

	// Update the layer metadata
	updatedLayerMD := metadata.Buildpacks[bpIdx].Layers[layerName]
	updatedLayerMD.SHA = op.patchLayerMD.SHA
	updatedLayerMD.Data = op.patchLayerMD.Data
	metadata.Buildpacks[bpIdx].Layers[layerName] = updatedLayerMD

	p.Logger.Infof("Patched layer %s:%s (SHA: %s -> %s)",
		op.match.BuildpackID, layerName, originalSHA, op.patchLayerMD.SHA)

	return PatchResult{
		BuildpackID:   op.match.BuildpackID,
		BuildpackIdx:  bpIdx,
		LayerName:     layerName,
		OriginalSHA:   originalSHA,
		NewSHA:        op.patchLayerMD.SHA,
		NewLayerData:  op.patchLayerMD.Data,
		NewLayerFlags: op.patchLayerMD.LayerMetadataFile,
	}, nil
}
