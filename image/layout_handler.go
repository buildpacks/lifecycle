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

func FromLayoutPath(parentPath string) (v1.Image, string, error) {
	fis, err := os.ReadDir(parentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	if len(fis) == 0 {
		return nil, "", nil
	}
	if len(fis) > 1 {
		return nil, "", fmt.Errorf("expected directory %q to have only 1 item; found %d", parentPath, len(fis))
	}
	imagePath := filepath.Join(parentPath, fis[0].Name())
	layoutPath, err := layout.FromPath(imagePath)
	if err != nil {
		return nil, "", err
	}
	index, err := layoutPath.ImageIndex()
	if err != nil {
		return nil, "", err
	}
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, "", err
	}
	manifests := indexManifest.Manifests
	if len(manifests) != 1 {
		return nil, "", fmt.Errorf("expected image index at %q to have only 1 manifest; found %d", imagePath, len(manifests))
	}
	manifest := manifests[0]
	image, err := layoutPath.Image(manifest.Digest)
	if err != nil {
		return nil, "", err
	}
	return image, imagePath, nil
}
