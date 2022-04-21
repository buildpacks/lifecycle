package io

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func Copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func RecursiveCopy(src, dst string) error {
	if err := os.MkdirAll(dst, 0777); err != nil {
		return errors.Wrap(err, "creating destination directory")
	}
	fis, err := ioutil.ReadDir(src)
	if err != nil {
		return errors.Wrap(err, "reading source directory")
	}
	for _, fi := range fis {
		if fi.Mode().IsRegular() {
			if err := Copy(filepath.Join(src, fi.Name()), filepath.Join(dst, fi.Name())); err != nil {
				return errors.Wrap(err, "copying file")
			}
		}
		if fi.IsDir() {
			if err := os.Mkdir(filepath.Join(dst, fi.Name()), fi.Mode()); err != nil {
				return errors.Wrap(err, "creating child directory")
			}
			if err := RecursiveCopy(filepath.Join(src, fi.Name()), filepath.Join(dst, fi.Name())); err != nil {
				return err
			}
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filepath.Join(src, fi.Name()))
			if err != nil {
				return errors.Wrap(err, "reading symlink")
			}
			if filepath.IsAbs(target) {
				return errors.New("symlinks cannot be absolute")
			}
			if err := os.Symlink(target, filepath.Join(dst, fi.Name())); err != nil {
				return errors.New("error creating symlink")
			}
		}
	}
	return nil
}
