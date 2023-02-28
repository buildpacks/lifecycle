package image

import (
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layout"
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
