package layout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildpacks/imgutil"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/pkg/errors"
)

var _ imgutil.Image = (*Image)(nil)

type Image struct {
	path                string
	imageRef            string
	underlyingImage     v1.Image
	underlyingImagePath string
	config              *v1.ConfigFile
	prevImage           *Image
	additionalLayers    []layerInfo
}

type layerInfo struct {
	diffID string
	path   string
}

type ImageOption func(*options) error

type options struct {
	baseImagePath string
	prevImagePath string
	imageRef      string
}

type IDIdentifier struct {
	ImageID string
}

func (i IDIdentifier) String() string {
	return i.ImageID
}

// WithPreviousImage loads an existing image as a source for reusable layers.
// Use with ReuseLayer().
// Ignored if image is not found.
func WithPreviousImage(path string) ImageOption {
	return func(i *options) error {
		i.prevImagePath = path
		return nil
	}
}

// FromBaseImage loads an existing image as the config and layers for the new image.
// Ignored if image is not found.
func FromBaseImage(path string) ImageOption {
	return func(i *options) error {
		i.baseImagePath = path
		return nil
	}
}

func WithImageRef(imageRef string) ImageOption {
	return func(i *options) error {
		i.imageRef = imageRef
		return nil
	}
}

func NewImage(path string, ops ...ImageOption) (*Image, error) {
	imageOpts := &options{}
	for _, op := range ops {
		if err := op(imageOpts); err != nil {
			return nil, err
		}
	}

	image := &Image{
		path:            path,
		imageRef:        imageOpts.imageRef,
		underlyingImage: EmptyImage,
		config: &v1.ConfigFile{
			Config: v1.Config{
				Labels: map[string]string{},
			},
		},
	}

	if ImageExists(imageOpts.baseImagePath) {
		err := processBaseImagePath(image, imageOpts.baseImagePath)
		if err != nil {
			return nil, err
		}
	}

	if ImageExists(imageOpts.prevImagePath) {
		err := processPrevImagePath(image, imageOpts.prevImagePath)
		if err != nil {
			return nil, err
		}
	}
	return image, nil
}

func loadImage(path string) (*Image, error) {
	p, err := layout.FromPath(path)
	if err != nil {
		return nil, errors.Wrap(err, "loading layout image")
	}

	index, err := p.ImageIndex()
	if err != nil {
		return nil, errors.Wrap(err, "reading index")
	}

	// TODO: check mediaType
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, errors.Wrap(err, "reading index manifest")
	}

	// TODO: Find based on platform (os/arch)
	if len(manifest.Manifests) == 0 {
		return nil, errors.New("no image manifest found")
	}

	image, err := index.Image(manifest.Manifests[0].Digest)
	if err != nil {
		return nil, errors.Wrap(err, "lookup image")
	}

	config, err := image.ConfigFile()
	if err != nil {
		return nil, errors.Wrap(err, "reading config")
	}

	return &Image{
		underlyingImage: image,
		path:            path,
		config:          config,
	}, nil
}

func processBaseImagePath(image *Image, path string) error {
	baseImage, err := loadImage(path)
	if err != nil {
		return errors.Wrap(err, "process base image")
	}
	image.config = baseImage.config.DeepCopy()
	image.underlyingImage = baseImage.underlyingImage
	image.underlyingImagePath = path
	return nil
}

func processPrevImagePath(image *Image, path string) error {
	prevImage, err := loadImage(path)
	if err != nil {
		return errors.Wrap(err, "process previous image")
	}
	image.prevImage = prevImage
	return nil
}

// Name returns the full path of the location the image is stored
func (i *Image) Name() string {
	return i.path
}

// TODO: What's the right implementation here?
func (i *Image) Rename(name string) {
	i.path = name
}

func (i *Image) Label(key string) (string, error) {
	return i.config.Config.Labels[key], nil
}

func (i *Image) Labels() (map[string]string, error) {
	return i.config.Config.Labels, nil
}

func (i *Image) SetLabel(key string, val string) error {
	i.config.Config.Labels[key] = val
	return nil
}

func (i *Image) RemoveLabel(key string) error {
	delete(i.config.Config.Labels, key)
	return nil
}

func (i *Image) Env(key string) (string, error) {
	for _, envVar := range i.config.Config.Env {
		parts := strings.Split(envVar, "=")
		if parts[0] == key {
			return parts[1], nil
		}
	}
	return "", nil
}

func (i *Image) SetEnv(key string, val string) error {
	i.config.Config.Env = append(i.config.Config.Env, key+"="+val)
	return nil
}

func (i *Image) Entrypoint() ([]string, error) {
	return i.config.Config.Entrypoint, nil
}

func (i *Image) SetEntrypoint(entrypoint ...string) error {
	i.config.Config.Entrypoint = entrypoint
	return nil
}

func (i *Image) SetWorkingDir(path string) error {
	i.config.Config.WorkingDir = path
	return nil
}

func (i *Image) SetCmd(cmd ...string) error {
	i.config.Config.Cmd = cmd
	return nil
}

func (i *Image) SetOS(os string) error {
	i.config.OS = os
	return nil
}

func (i *Image) SetOSVersion(version string) error {
	i.config.OSVersion = version
	return nil
}

func (i *Image) SetArchitecture(arch string) error {
	i.config.Architecture = arch
	return nil
}

// TODO: Implement
func (i *Image) Rebase(string, imgutil.Image) error {
	return nil
}

// AddLayer adds an uncompressed tarred layer to the image
func (i *Image) AddLayer(path string) error {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return errors.Wrapf(err, "AddLayer: open layer: %s", path)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return errors.Wrapf(err, "AddLayer: calculate checksum: %s", path)
	}
	diffID := "sha256:" + hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
	return i.AddLayerWithDiffID(path, diffID)
}

func (i *Image) AddLayerWithDiffID(path, diffID string) error {
	i.additionalLayers = append(i.additionalLayers, layerInfo{
		diffID: diffID,
		path:   path,
	})

	return nil
}

func (i *Image) ReuseLayer(diffID string) error {
	if i.prevImage == nil {
		return errors.New("failed to reuse layer because no previous image was provided")
	}

	if !i.prevImage.Found() {
		return errors.Errorf("failed to reuse layer because previous image was not found at %s", i.prevImage.path)
	}

	for _, hash := range i.prevImage.config.RootFS.DiffIDs {
		if hash.String() == diffID {
			return i.AddLayerWithDiffID(fullLayerPath(i.prevImage.path, hash), diffID)
		}
	}

	return nil
}

// TODO: Verify implementation
func fullLayerPath(imagePath string, diffID v1.Hash) string {
	return filepath.Join(imagePath, "blobs", diffID.String())
}

// TopLayer returns the diffID for the top layer
func (i *Image) TopLayer() (string, error) {
	if len(i.additionalLayers) > 0 {
		return i.additionalLayers[len(i.additionalLayers)-1].diffID, nil
	}

	if len(i.config.RootFS.DiffIDs) > 0 {
		return i.config.RootFS.DiffIDs[len(i.config.RootFS.DiffIDs)-1].String(), nil
	}

	// TODO: what do we return when not found? error?
	return "", nil
}

// Save saves the image as `Name()` and any additional names provided to this method.
func (i *Image) Save(additionalNames ...string) error {
	var (
		image = EmptyImage
		err   error
	)

	image, err = mutate.Config(image, i.config.Config)
	if err != nil {
		return errors.Wrap(err, "set config")
	}

	_, err = layout.Write(i.Name(), empty.Index)

	if i.underlyingImage != nil {

		runLayers, _ := i.underlyingImage.Layers()
		layerBlobsDir := filepath.Join(i.path, "blobs", "sha256")
		err = os.MkdirAll(layerBlobsDir, 0755)
		if err != nil {
			return errors.Wrap(err, "appending run image layer 1")
		}
		start := time.Now()
		for _, runLayer := range runLayers {
			blob, err := runLayer.Digest()
			fmt.Println(blob.String())
			if err != nil {
				return errors.Wrap(err, "appending run image layer 2")
			}
			b := strings.Split(blob.String(), ":")
			layerPathRef := filepath.Join("..", "..", "..", i.imageRef, "blobs", b[0], b[1])
			layerSymLink := filepath.Join(layerBlobsDir, b[1])
			os.Symlink(layerPathRef, layerSymLink)
			image, err = mutate.AppendLayers(image, runLayer)
			if err != nil {
				return errors.Wrap(err, "appending layer")
			}
		}
		elapsed := time.Since(start)
		fmt.Printf("Adding run image layers took %s\n", elapsed)
	}

	// FIXME: This still produces a gzip (with no actual compression) based on the current GGCR implementation.
	// We would need to PR a change to skip gunzip packaging all together and update the mediatype.
	// See: https://github.com/google/go-containerregistry/blob/c061b3f39cff652d18f95ee23ebfd39cb3f5ee89/pkg/v1/tarball/layer.go#L85
	start := time.Now()
	for _, layerInfo := range i.additionalLayers {
		fmt.Printf("Loading layer from path: %s and diffID: %s\n", layerInfo.path, layerInfo.diffID)
		file, err := os.Open(layerInfo.path)
		if err != nil {
			return errors.Wrapf(err, "creating layer from %s", layerInfo.path)
		}
		layer := stream.NewLayer(file)
		if err != nil {
			return errors.Wrapf(err, "creating layer from %s", layerInfo.path)
		}
		image, err = appendOCILayer(image, layer)
		if err != nil {
			return errors.Wrapf(err, "appending layer %s", layerInfo.path)
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("Adding layers to image took %s\n", elapsed)

	if err != nil {
		return errors.Wrap(err, "creating layout dir")
	}

	path := layout.Path(i.Name())
	var annotations map[string]string
	if len(additionalNames) > 0 {
		annotations = map[string]string{"org.opencontainers.image.ref.name": additionalNames[0]}
	}

	start = time.Now()
	err = path.AppendImage(image, layout.WithAnnotations(annotations))
	elapsed = time.Since(start)
	fmt.Printf("Writing image to disk took %s\n", elapsed)
	if err != nil {
		return errors.Wrap(err, "append image")
	}
	return nil
}

// Found tells whether the image exists in the repository by `Name()`.
func (i *Image) Found() bool {
	if _, err := os.Stat(i.path); os.IsNotExist(err) {
		return false
	}
	return true
}

func (i *Image) GetLayer(diffID string) (io.ReadCloser, error) {
	for _, layerInfo := range i.additionalLayers {
		if layerInfo.diffID == diffID {
			return os.Open(layerInfo.path)
		}
	}

	hash, err := v1.NewHash(diffID)
	if err != nil {
		return nil, errors.Wrap(err, "parsing diffID")
	}

	layer, err := i.underlyingImage.LayerByDiffID(hash)
	if err != nil {
		return nil, errors.Wrap(err, "reading layer")
	}

	return layer.Uncompressed()
}

func (i *Image) Delete() error {
	return os.RemoveAll(i.path)
}

func (i *Image) CreatedAt() (time.Time, error) {
	return time.Now(), nil
}

// Each image's ID is given by the SHA256 hash of its configuration JSON. It is represented as a hexadecimal encoding of 256 bits,
// e.g., sha256:a9561eb1b190625c9adb5a9513e72c4dedafc1cb2d4c5236c9a6957ec7dfd5a9.
func (i *Image) Identifier() (imgutil.Identifier, error) {
	return IDIdentifier{
		ImageID: "sha256:" + asSha256(i.config),
	}, nil
}

func (i *Image) OS() (string, error) {
	return i.config.OS, nil
}

func (i *Image) OSVersion() (string, error) {
	return i.config.OSVersion, nil
}

func (i *Image) Architecture() (string, error) {
	return i.config.Architecture, nil
}

// ManifestSize returns the size of the manifest. If a manifest doesn't exist, it returns 0.
func (i *Image) ManifestSize() (int64, error) {
	// TODO: Compute
	return 0, nil
}

func (i *Image) WorkingDir() (string, error) {
	panic("implement me")
}

func asSha256(o interface{}) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", o)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func pathExists(path string) bool {
	if path != "" {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

func ImageExists(path string) bool {
	if !pathExists(path) {
		return false
	}
	index := filepath.Join(path, "index.json")
	if _, err := os.Stat(index); os.IsNotExist(err) {
		return false
	}
	return true
}

// This method is a workaround to override the Media Type of the layer to OCILayer
func appendOCILayer(image v1.Image, layer v1.Layer) (v1.Image, error) {
	additions := make([]mutate.Addendum, 0)
	additions = append(additions, mutate.Addendum{
		MediaType: types.OCILayer,
		Layer:     layer,
	})
	image, err := mutate.Append(image, additions...)
	if err != nil {
		return nil, err
	}
	return image, nil
}
