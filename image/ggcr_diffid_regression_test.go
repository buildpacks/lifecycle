package image

import (
	"archive/tar"
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func TestAppendLayerWithEmptyConfigMediaType(t *testing.T) {
	var contents bytes.Buffer
	tw := tar.NewWriter(&contents)
	if err := tw.WriteHeader(&tar.Header{Name: "app/file", Mode: 0o644, Size: int64(len("contents"))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("contents")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	layer, err := tarball.LayerFromReader(bytes.NewReader(contents.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	expectedDiffID, err := layer.DiffID()
	if err != nil {
		t.Fatal(err)
	}
	compressedDigest, err := layer.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if expectedDiffID == compressedDigest {
		t.Fatal("test layer must have different compressed and uncompressed digests")
	}

	base := mutate.ConfigMediaType(mutate.MediaType(empty.Image, types.OCIManifestSchema1), "")
	image, err := mutate.AppendLayers(base, layer)
	if err != nil {
		t.Fatal(err)
	}
	config, err := image.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := image.Manifest()
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]v1.Hash{compressedDigest}, []v1.Hash{manifest.Layers[0].Digest}); diff != "" {
		t.Fatalf("manifest layer digest mismatch (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff([]v1.Hash{expectedDiffID}, config.RootFS.DiffIDs); diff != "" {
		t.Fatalf("RootFS.DiffIDs mismatch (-want, +got):\n%s", diff)
	}
}
