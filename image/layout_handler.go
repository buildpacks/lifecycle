package image

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layout"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const LayoutKind = "layout"

type LayoutHandler struct {
	layoutDir string
}

func (h *LayoutHandler) InitImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	path, err := h.parseRef(imageRef)
	if err != nil {
		return nil, err
	}
	return layout.NewImage(path, layout.FromBaseImagePath(path))
}

func (h *LayoutHandler) Kind() string {
	return LayoutKind
}

func (h *LayoutHandler) parseRef(imageRef string) (string, error) {
	path, err := layout.ParseRefToPath(imageRef)
	if err != nil {
		return "", err
	}
	return filepath.Join(h.layoutDir, path), nil
}

// helpers

func FromLayoutPath(parentPath string) (v1.Image, error) {
	fis, err := os.ReadDir(parentPath)
	if err != nil {
		return nil, err
	}
	if len(fis) > 1 { // TODO: this is weird
		return nil, fmt.Errorf("expected directory %q to have only 1 item; found %d", parentPath, len(fis))
	}
	imageName := fis[0].Name()
	layoutPath, err := layout.FromPath(filepath.Join(parentPath, imageName))
	if err != nil {
		return nil, err
	}
	index, err := layoutPath.ImageIndex()
	if err != nil {
		return nil, err
	}
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}
	manifests := indexManifest.Manifests
	if len(manifests) != 1 {
		return nil, fmt.Errorf("expected image %q to have only 1 manifest; found %d", imageName, len(manifests))
	}
	manifest := manifests[0]
	return layoutPath.Image(manifest.Digest)
}
