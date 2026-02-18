package files

// LayerPatchesFile contains the configuration for patching buildpack-contributed layers during rebase.
// This is an experimental feature that allows selective patching of layers from an OCI patch image.
type LayerPatchesFile struct {
	Patches []LayerPatch `json:"patches"`
}

// LayerPatch describes a single layer patch operation.
type LayerPatch struct {
	// Buildpack matches the buildpack key in io.buildpacks.lifecycle.metadata label.
	// This corresponds to buildpacks[].key in the metadata.
	Buildpack string `json:"buildpack"`

	// Layer matches a layer name within the buildpack's layers.
	// This corresponds to buildpacks[].layers key in the metadata.
	Layer string `json:"layer"`

	// Data contains dot-notation selectors with optional glob support for matching layer data.
	// For example, {"artifact.version": "1.2.*"} will match layers where the nested
	// artifact.version field matches the glob pattern "1.2.*".
	Data map[string]string `json:"data,omitempty"`

	// PatchImage is the primary OCI image reference containing the patch layers.
	PatchImage string `json:"patch-image"`

	// PatchImageMirrors are fallback image references to try if the primary fails.
	PatchImageMirrors []string `json:"patch-image.mirrors,omitempty"`
}
