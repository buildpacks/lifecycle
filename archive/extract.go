package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type PathMode struct {
	Path string
	Mode os.FileMode
}

func Extract(tr TarReader) error {
	// Avoid umask from changing the file permissions in the tar file.
	umask := setUmask(0)
	defer setUmask(umask)

	buf := make([]byte, 32*32*1024)
	dirsFound := make(map[string]bool)

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

		switch hdr.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(hdr.Name); os.IsNotExist(err) {
				pathMode := PathMode{hdr.Name, hdr.FileInfo().Mode()}
				pathModes = append(pathModes, pathMode)
			}
			if err := os.MkdirAll(hdr.Name, os.ModePerm); err != nil {
				return err
			}
			dirsFound[hdr.Name] = true

		case tar.TypeReg, tar.TypeRegA:
			dirPath := filepath.Dir(hdr.Name)
			if !dirsFound[dirPath] {
				if _, err := os.Stat(dirPath); os.IsNotExist(err) {
					if err := os.MkdirAll(dirPath, applyUmask(os.ModePerm, umask)); err != nil {
						return err
					}
					dirsFound[dirPath] = true
				}
			}

			if err := writeFile(tr, hdr.Name, hdr.FileInfo().Mode(), buf); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, hdr.Name); err != nil {
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
