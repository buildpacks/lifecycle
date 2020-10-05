package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/archive"
)

var (
	archivePath    string
	descriptorPath string
	inputDir       string
	version        string
	exitCode       int = 2
)

// Write contents of inputDir to archive at archivePath
func main() {
	flag.StringVar(&archivePath, "archivePath", "", "path to output")
	flag.StringVar(&descriptorPath, "descriptorPath", "", "path to lifecycle descriptor file")
	flag.StringVar(&inputDir, "inputDir", "", "dir to create package from")
	flag.StringVar(&version, "version", "", "lifecycle version")

	flag.Parse()
	if archivePath == "" || inputDir == "" || version == "" {
		flag.Usage()
		os.Exit(1)
	}

	if err := doPackage(); err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}
}

func doPackage() error {
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to open -archivePath %s", archivePath))
	}
	defer f.Close()

	zw := gzip.NewWriter(f)
	defer zw.Close()

	tw := archive.NewNormalizingTarWriter(tar.NewWriter(zw))
	tw.WithUID(0)
	tw.WithGID(0)
	defer tw.Close()

	templateContents, err := ioutil.ReadFile(descriptorPath)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to read descriptor file %s", descriptorPath))
	}

	descriptorContents, err := fillTemplate(templateContents, map[string]interface{}{"lifecycle_version": version})
	if err != nil {
		return errors.Wrap(err, "Failed to fill template")
	}

	descriptorTemplateInfo, err := os.Stat(descriptorPath)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to stat descriptor template file %s", descriptorPath))
	}

	tempDir, err := ioutil.TempDir("", "lifecycle-descriptor")
	if err != nil {
		return errors.Wrap(err, "Failed to create a temp directory")
	}

	tempFile, err := os.Create(filepath.Join(tempDir, "lifecycle.toml"))
	if err != nil {
		return errors.Wrap(err, "Failed to create a temp file")
	}

	err = ioutil.WriteFile(tempFile.Name(), descriptorContents, descriptorTemplateInfo.Mode())
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to write descriptor contents to file %s", tempFile.Name()))
	}

	err = os.Chdir(tempDir)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to switch directories to %s", tempDir))
	}

	descriptorInfo, err := os.Stat(tempFile.Name())
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to stat descriptor file %s", tempFile.Name()))
	}

	err = archive.AddFileToArchive(tw, "lifecycle.toml", descriptorInfo)
	if err != nil {
		return errors.Wrap(err, "Failed to write descriptor to archive")
	}

	err = os.Chdir(filepath.Dir(inputDir))
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to switch directories to %s", filepath.Dir(inputDir)))
	}

	err = archive.AddDirToArchive(tw, filepath.Base(inputDir))
	if err != nil {
		return errors.Wrap(err, "Failed to write dir to archive")
	}

	return nil
}

func fillTemplate(templateContents []byte, data map[string]interface{}) ([]byte, error) {
	tpl, err := template.New("").Parse(string(templateContents))
	if err != nil {
		return []byte{}, err
	}

	var templatedContent bytes.Buffer
	err = tpl.Execute(&templatedContent, data)
	if err != nil {
		return []byte{}, err
	}

	return templatedContent.Bytes(), nil
}
