package archive

import (
	"archive/tar"
	"time"
)

type TarWriter interface {
	WriteHeader(hdr *tar.Header) error
	Write(b []byte) (int, error)
	Close() error
}

type NormalizedTarWriter struct {
	TarWriter
	headerOpts []HeaderOpt
}

type HeaderOpt func(header *tar.Header) *tar.Header

func (tw *NormalizedTarWriter) WithUID(uid int) {
	tw.headerOpts = append(tw.headerOpts, func(header *tar.Header) *tar.Header {
		header.Uid = uid
		return header
	})
}

func (tw *NormalizedTarWriter) WithGID(gid int) {
	tw.headerOpts = append(tw.headerOpts, func(header *tar.Header) *tar.Header {
		header.Gid = gid
		return header
	})
}

func NewNormalizedTarWriter(tw TarWriter) *NormalizedTarWriter {
	return &NormalizedTarWriter{tw, []HeaderOpt{}}
}

func (tw *NormalizedTarWriter) WriteHeader(hdr *tar.Header) error {
	for _, opt := range tw.headerOpts {
		hdr = opt(hdr)
	}
	hdr.ModTime = time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)
	hdr.Uname = ""
	hdr.Gname = ""
	return tw.TarWriter.WriteHeader(hdr)
}
