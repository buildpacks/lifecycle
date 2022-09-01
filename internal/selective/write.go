package selective

import (
	"bytes"
	"io/ioutil"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
)

// TODO: test the functions in this file

// AppendImage mimics GGCR's `layout` AppendImage in that it appends an image to a `layout.Path`,
// but the image appended does not include any layers in the `blobs` directory.
// The returned image will error when Layers(), LayerByDigest(), or LayerByDiffID() are called.
// This is useful when we need to satisfy the v1.Image interface but do not need to access any layers, such as when extending
// base images with kaniko.
func (l Path) AppendImage(img v1.Image) error { // FIXME: add the ability to pass image options
	if err := l.WriteImage(img); err != nil {
		return err
	}

	mt, err := img.MediaType()
	if err != nil {
		return err
	}

	d, err := img.Digest()
	if err != nil {
		return err
	}

	manifest, err := img.RawManifest()
	if err != nil {
		return err
	}

	desc := v1.Descriptor{
		MediaType: mt,
		Size:      int64(len(manifest)),
		Digest:    d,
	}

	return l.AppendDescriptor(desc)
}

// WriteImage mimics GGCR's `layout` WriteImage in that it writes an image config and manifest,
// but it does not write any layers in the `blobs` directory.
// The returned image will error when Layers(), LayerByDigest(), or LayerByDiffID() are called.
// This is useful when we need to satisfy the v1.Image interface but do not need to access any layers,
// such as when extending base images with kaniko.
func (l Path) WriteImage(img v1.Image) error {
	// Write the config.
	cfgName, err := img.ConfigName()
	if err != nil {
		return err
	}
	cfgBlob, err := img.RawConfigFile()
	if err != nil {
		return err
	}
	if err := l.WriteBlob(cfgName, ioutil.NopCloser(bytes.NewReader(cfgBlob))); err != nil {
		return err
	}

	// Write the img manifest.
	d, err := img.Digest()
	if err != nil {
		return err
	}
	manifest, err := img.RawManifest()
	if err != nil {
		return err
	}
	return l.WriteBlob(d, ioutil.NopCloser(bytes.NewReader(manifest)))
}

func Write(path string, ii v1.ImageIndex) (Path, error) {
	layoutPath, err := layout.Write(path, ii)
	if err != nil {
		return Path{}, err
	}

	return Path{Path: layoutPath}, nil
}
