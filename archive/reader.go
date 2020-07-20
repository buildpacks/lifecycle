package archive

import (
	"archive/tar"
	"path/filepath"
	"strings"
)

type TarReader interface {
	Next() (*tar.Header, error)
	Read(b []byte) (int, error)
}

type NormalizingTarReader struct {
	TarReader
	headerOpts    []HeaderOpt
	excludedPaths []string
}

func (tr *NormalizingTarReader) Strip(prefix string) {
	tr.headerOpts = append(tr.headerOpts, func(header *tar.Header) *tar.Header {
		header.Name = strings.TrimPrefix(header.Name, prefix)
		return header
	})
}

func (tr *NormalizingTarReader) ExcludePaths(paths []string) {
	tr.excludedPaths = paths
}

func (tr *NormalizingTarReader) ToWindows() {
	tr.headerOpts = append(tr.headerOpts, func(hdr *tar.Header) *tar.Header {
		hdr.Name = filepath.FromSlash(hdr.Name)
		return hdr
	})
}

func (tr *NormalizingTarReader) PrependDir(dir string) {
	tr.headerOpts = append(tr.headerOpts, func(hdr *tar.Header) *tar.Header {
		hdr.Name = filepath.Join(dir, hdr.Name)
		return hdr
	})
}

func NewNormalizingTarReader(tr TarReader) *NormalizingTarReader {
	return &NormalizingTarReader{TarReader: tr}
}

func (tr *NormalizingTarReader) Next() (*tar.Header, error) {
	hdr, err := tr.TarReader.Next()
	if err != nil {
		return nil, err
	}
	for _, excluded := range tr.excludedPaths {
		if strings.HasPrefix(hdr.Name, excluded) {
			return tr.Next() // If path is excluded move on to the next entry
		}
	}
	for _, opt := range tr.headerOpts {
		hdr = opt(hdr)
	}
	if hdr.Name == "" {
		return tr.Next() // If entire path is stripped move on to the next entry
	}
	return hdr, nil
}
