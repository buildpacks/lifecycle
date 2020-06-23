package image

import (
	"archive/tar"
	"io"

	"github.com/buildpacks/imgutil/layer"

	"github.com/buildpacks/lifecycle/archive"
)

type LayerWriterFactory struct {
}

func NewLayerWriterFactory() (*LayerWriterFactory, error) {
	return &LayerWriterFactory{}, nil
}

func (f *LayerWriterFactory) NewWriter(fileWriter io.Writer) archive.TarWriter {
	if archive.LayerOS() == "windows" {
		return layer.NewWindowsWriter(fileWriter)
	}

	// Linux images use tar.Writer
	return tar.NewWriter(fileWriter)
}
