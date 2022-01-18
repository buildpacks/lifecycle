package layout

import (
	"bytes"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// Image is a singleton empty image, think: FROM scratch.
var EmptyImage, _ = partial.CompressedToImage(emptyImage{})

type emptyImage struct{}

func (i emptyImage) Manifest() (*v1.Manifest, error) {
	b, err := i.RawConfigFile()
	if err != nil {
		return nil, err
	}

	cfgHash, cfgSize, err := v1.SHA256(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	m := &v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Config: v1.Descriptor{
			MediaType: types.OCIConfigJSON,
			Size:      cfgSize,
			Digest:    cfgHash,
		},
	}
	return m, nil
}

func (i emptyImage) RawManifest() ([]byte, error) {
	return partial.RawManifest(i)
}

func (i emptyImage) LayerByDigest(hash v1.Hash) (partial.CompressedLayer, error) {
	return nil, fmt.Errorf("LayerByDigest(%s): empty image", hash)
}

// MediaType implements partial.UncompressedImageCore.
func (i emptyImage) MediaType() (types.MediaType, error) {
	return types.OCIManifestSchema1, nil
}

// RawConfigFile implements partial.UncompressedImageCore.
func (i emptyImage) RawConfigFile() ([]byte, error) {
	return partial.RawConfigFile(i)
}

// ConfigFile implements v1.Image.
func (i emptyImage) ConfigFile() (*v1.ConfigFile, error) {
	return &v1.ConfigFile{
		Architecture: "amd64",
		OS:           "linux",
		RootFS: v1.RootFS{
			// Some clients check this.
			Type: "layers",
		},
		Config: v1.Config{
			Labels: map[string]string{},
		},
	}, nil
}

func (i emptyImage) LayerByDiffID(h v1.Hash) (partial.UncompressedLayer, error) {
	return nil, fmt.Errorf("LayerByDiffID(%s): empty image", h)
}
