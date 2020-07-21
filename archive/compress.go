package archive

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
)

type PathInfo struct {
	Path string
	Info os.FileInfo
}

func WriteFilesToArchive(tw TarWriter, files []PathInfo) error {
	for _, file := range files {
		if err := AddFileToArchive(tw, file.Path, file.Info); err != nil {
			return err
		}
	}
	return nil
}

func AddFileToArchive(tw TarWriter, path string, fi os.FileInfo) error {
	if fi.Mode()&os.ModeSocket != 0 {
		return nil
	}
	header, err := tar.FileInfoHeader(fi, "")
	if err != nil {
		return err
	}
	header.Name = path

	if fi.Mode()&os.ModeSymlink != 0 {
		var err error
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		header.Linkname = target
		addSysAttributes(header, fi)
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if fi.Mode().IsRegular() {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}
	}
	return nil
}

func AddDirToArchive(tw TarWriter, srcDir string) error {
	srcDir = filepath.Clean(srcDir)

	return filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return AddFileToArchive(tw, file, fi)
	})
}
