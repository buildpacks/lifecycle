package image

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layout"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const LayoutKind = "layout"

type LayoutHandler struct {
	layoutDir          string
	keychain           authn.Keychain
	insecureRegistries []string
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

// InitRemoteImage TODO
func (h *LayoutHandler) InitRemoteImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	options := []imgutil.ImageOption{
		remote.FromBaseImage(imageRef),
	}

	options = append(options, GetInsecureOptions(h.insecureRegistries)...)

	return remote.NewImage(
		imageRef,
		h.keychain,
		options...,
	)
}

func (h *LayoutHandler) Kind() string {
	return LayoutKind
}

func (h *LayoutHandler) parseRef(imageRef string) (string, error) {
	if !strings.HasPrefix(imageRef, h.layoutDir) {
		path, err := layout.ParseRefToPath(imageRef)
		if err != nil {
			return "", err
		}
		return filepath.Join(h.layoutDir, path), nil
	}
	return imageRef, nil
}

// helpers

// FromLayoutPath takes a path to a directory (such as <layers>/extended/run) containing a single image in "sparse" OCI layout format,
// and returns a v1.Image along with the path of the image (such as <layers>/extended/run/sha256:<sha256>)
// or an error if the image cannot be loaded.
// The path is helpful for locating the image when we only know the digest of the config, such as for local images.
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
