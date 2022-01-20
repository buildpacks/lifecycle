package util

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const root = "/"

func GetCNBEnvVar() map[string]string {
	kvs := map[string]string{}
	envs := os.Environ()

	for _, env := range envs {
		if strings.Contains(env, "CNB") {
			str := strings.Split(env, "=")
			kvs[str[0]] = str[1]
		}
	}
	return kvs
}

func GetValFromEnVar(envVar string) (val string) {
	val, ok := os.LookupEnv(envVar)
	if !ok {
		logrus.Debugf("%s not set", envVar)
		return ""
	} else {
		logrus.Debugf("%s=%s", envVar, val)
		return val
	}
}

func ReadFileContent(f *os.File) {
	data, err := ioutil.ReadFile(f.Name())
	if err != nil {
		logrus.Errorf("Failed reading data from file: %s", err)
	}
	logrus.Debugf("\nFile Name: %s", f.Name())
	logrus.Debugf("\nData: %s", data)
}

// File copies a single file from src to dst
func File(src, dst string) error {
	var err error
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		return err
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}
	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}
	return os.Chmod(dst, srcinfo.Mode())
}

func Dir(src string, dst string) error {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = Dir(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		} else {
			if err = File(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		}
	}
	return nil
}

func ReadFilesFromPath(path string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}

	for _, file := range files {
		fmt.Println(file.Name(), file.IsDir())
	}
	return nil
}

func FindFiles(filesToSearch []string) error {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		// TODO. Avoid to hard code the path to ignore
		if strings.HasPrefix(filepath.ToSlash(path), "/proc") {
			return nil
		}

		if err != nil {
			fmt.Println(err)
			return nil
		}

		for _, s := range filesToSearch {
			logrus.Tracef("File searched is : %s", info.Name())
			if !info.IsDir() && info.Name() == s {
				files = append(files, path)
			}
		}
		return nil
	})

	if err != nil {
		panic(err)
	}

	for _, file := range files {
		logrus.Infof("File found: %s", file)
	}
	return nil
}

func UnGzip(r io.Reader) (gzf io.Reader, err error) {
	logrus.Info("Creating a gzip reader")
	gzf, err = gzip.NewReader(r)
	return gzf, nil
}

func UnTar(tarFilePath string) (tarR io.Reader, err error) {
	logrus.Infof("Opening the tar file: %s", tarFilePath)
	f, err := os.Open(tarFilePath)
	if err != nil {
		panic(err)
	}
	logrus.Infof("Creating a reader for: %s", f.Name())
	tarR = tar.NewReader(f)
	return tarR, nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func FilterFiles(root, ext string) []string {
	var files []string
	filepath.Walk(root, func(path string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		logrus.Infof("Cache file: %s, ext: %s",info.Name(),filepath.Ext(path) )
		if !info.IsDir() && filepath.Ext(path) == ext {
			files = append(files, path)
		}
		return nil
	})
	logrus.Infof("Files found: %d",len(files))
	return files
}
