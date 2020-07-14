package archive

import (
	"archive/tar"
	"fmt"
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
	var target string
	if fi.Mode()&os.ModeSymlink != 0 {
		var err error
		target, err = os.Readlink(path)
		if err != nil {
			return err
		}
	}
	header, err := tar.FileInfoHeader(fi, target)
	if err != nil {
		return err
	}
	header.Name = path

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

type PathMode struct {
	Path string
	Mode os.FileMode
}

func Untar(r io.Reader, dest string) error {
	// Avoid umask from changing the file permissions in the tar file.
	umask := setUmask(0)
	defer setUmask(umask)

	buf := make([]byte, 32*32*1024)
	dirsFound := make(map[string]bool)

	tr := tar.NewReader(r)
	var pathModes []PathMode
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			for _, pathMode := range pathModes {
				if err := os.Chmod(pathMode.Path, pathMode.Mode); err != nil {
					return err
				}
			}
			return nil
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(path); os.IsNotExist(err) {
				pathMode := PathMode{path, hdr.FileInfo().Mode()}
				pathModes = append(pathModes, pathMode)
			}
			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				return err
			}
			dirsFound[path] = true

		case tar.TypeReg, tar.TypeRegA:
			dirPath := filepath.Dir(path)
			if !dirsFound[dirPath] {
				if _, err := os.Stat(dirPath); os.IsNotExist(err) {
					if err := os.MkdirAll(dirPath, applyUmask(os.ModePerm, umask)); err != nil {
						return err
					}
					dirsFound[dirPath] = true
				}
			}

			if err := writeFile(tr, path, hdr.FileInfo().Mode(), buf); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown file type in tar %d", hdr.Typeflag)
		}
	}
}

func applyUmask(mode os.FileMode, umask int) os.FileMode {
	return os.FileMode(int(mode) &^ umask)
}

func writeFile(in io.Reader, path string, mode os.FileMode, buf []byte) error {
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.CopyBuffer(fh, in, buf)
	return err
}
