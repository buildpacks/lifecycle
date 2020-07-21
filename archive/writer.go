package archive

import (
	"archive/tar"
	"path/filepath"
	"strings"
	"time"
)

type TarWriter interface {
	WriteHeader(hdr *tar.Header) error
	Write(b []byte) (int, error)
	Close() error
}

type NormalizingTarWriter struct {
	TarWriter
	headerOpts []HeaderOpt
}

type HeaderOpt func(header *tar.Header) *tar.Header

func (tw *NormalizingTarWriter) WithUID(uid int) {
	tw.headerOpts = append(tw.headerOpts, func(hdr *tar.Header) *tar.Header {
		hdr.Uid = uid
		return hdr
	})
}

func (tw *NormalizingTarWriter) WithGID(gid int) {
	tw.headerOpts = append(tw.headerOpts, func(hdr *tar.Header) *tar.Header {
		hdr.Gid = gid
		return hdr
	})
}

func NewNormalizingTarWriter(tw TarWriter) *NormalizingTarWriter {
	return &NormalizingTarWriter{tw, []HeaderOpt{}}
}

func (tw *NormalizingTarWriter) WriteHeader(hdr *tar.Header) error {
	for _, opt := range tw.headerOpts {
		hdr = opt(hdr)
	}
	hdr.Name = filepath.ToSlash(strings.TrimPrefix(hdr.Name, filepath.VolumeName(hdr.Name)))
	hdr.ModTime = time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)
	hdr.Uname = ""
	hdr.Gname = ""
	return tw.TarWriter.WriteHeader(hdr)
}
