package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
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

	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher) // calculate the sha256 while writing to f

	zw := gzip.NewWriter(mw)

	tw := archive.NewNormalizingTarWriter(tar.NewWriter(zw))
	tw.WithUID(0)
	tw.WithGID(0)

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

	err = tw.Close()
	if err != nil {
		return errors.Wrap(err, "Failed to close tar writer")
	}

	err = zw.Close()
	if err != nil {
		return errors.Wrap(err, "Failed to close gzip writer")
	}

	err = f.Close()
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to close -archivePath %s", archivePath))
	}

	hashFileName := archivePath + ".sha256"
	hashFile, err := os.OpenFile(hashFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to open %s", hashFileName))
	}
	defer hashFile.Close()

	sha := hex.EncodeToString(hasher.Sum(nil))
	_, err = hashFile.Write([]byte(archivePath + "  " + sha + "\n"))
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to write sha256:%s to %s", sha, hashFileName))
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
