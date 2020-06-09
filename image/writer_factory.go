package image

import (
	"archive/tar"
	"io"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layer"

	"github.com/buildpacks/lifecycle/archive"
)

type LayerWriterFactory struct {
	os string
}

func NewLayerWriterFactory(image imgutil.Image) (*LayerWriterFactory, error) {
	os, err := image.OS()
	if err != nil {
		return nil, err
	}

	return &LayerWriterFactory{os: os}, nil
}

func (f *LayerWriterFactory) NewWriter(fileWriter io.Writer) archive.TarWriter {
	if f.os == "windows" {
		return layer.NewWindowsWriter(fileWriter)
	}

	// Linux images use tar.Writer
	return tar.NewWriter(fileWriter)
}
