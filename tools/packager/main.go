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

	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
	handle(err, fmt.Sprintf("Failed to open -archivePath %s: %s", archivePath, err))
	defer f.Close()

	zw := gzip.NewWriter(f)
	defer zw.Close()

	tw := archive.NewNormalizingTarWriter(tar.NewWriter(zw))
	tw.WithUID(0)
	tw.WithGID(0)
	defer tw.Close()

	templateContents, err := ioutil.ReadFile(descriptorPath)
	handle(err, fmt.Sprintf("Failed to read descriptor file %s: %s", descriptorPath, err))

	descriptorContents, err := fillTemplate(templateContents, map[string]interface{}{"lifecycle_version": version})
	handle(err, fmt.Sprintf("Failed to fill template: %s", err))

	descriptorTemplateInfo, err := os.Stat(descriptorPath)
	handle(err, fmt.Sprintf("Failed to stat descriptor template file %s: %s", descriptorPath, err))

	tempDir, err := ioutil.TempDir("", "lifecycle-descriptor")
	handle(err, fmt.Sprintf("Failed to create a temp directory: %s", err))

	tempFile, err := os.Create(filepath.Join(tempDir, "lifecycle.toml"))
	handle(err, fmt.Sprintf("Failed to create a temp file: %s", err))

	err = ioutil.WriteFile(tempFile.Name(), descriptorContents, descriptorTemplateInfo.Mode())
	handle(err, fmt.Sprintf("Failed to write descriptor contents to file %s: %s", tempFile.Name(), err))

	err = os.Chdir(tempDir)
	handle(err, fmt.Sprintf("Failed to switch directories to %s: %s", tempDir, err))

	descriptorInfo, err := os.Stat(tempFile.Name())
	handle(err, fmt.Sprintf("Failed to stat descriptor file %s: %s", tempFile.Name(), err))

	err = archive.AddFileToArchive(tw, "lifecycle.toml", descriptorInfo)
	handle(err, fmt.Sprintf("Failed to write descriptor to archive: %s", err))

	err = os.Chdir(filepath.Dir(inputDir))
	handle(err, fmt.Sprintf("Failed to switch directories to %s: %s", filepath.Dir(inputDir), err))

	err = archive.AddDirToArchive(tw, filepath.Base(inputDir))
	handle(err, fmt.Sprintf("Failed to write dir to archive: %s", err))
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

func handle(err error, msg string) {
	if err != nil {
		fmt.Println(msg)
		os.Exit(exitCode)
	}
	exitCode++
}
