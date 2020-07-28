package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildpacks/lifecycle/archive"
)

var (
	archivePath string
	inputDir    string
)

// Write contents of inputDir to archive at archivePath
func main() {
	flag.StringVar(&archivePath, "archivePath", "", "path to output ")
	flag.StringVar(&inputDir, "inputDir", "", "dir to create package from")

	flag.Parse()
	if archivePath == "" || inputDir == "" {
		flag.Usage()
		os.Exit(1)
	}

	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0777)
	if err != nil {
		fmt.Printf("Failed to open -archivePath %s: %s", archivePath, err)
		os.Exit(2)
	}
	defer f.Close()
	zw := gzip.NewWriter(f)
	defer zw.Close()
	tw := tar.NewWriter(zw)
	defer tw.Close()

	if err := os.Chdir(filepath.Dir(inputDir)); err != nil {
		fmt.Printf("Failed to switch directories to %s: %s", filepath.Dir(inputDir), err)
		os.Exit(3)
	}

	if err := archive.AddDirToArchive(tw, filepath.Base(inputDir)); err != nil {
		fmt.Printf("Failed to write dir to arichive: %s", err)
		os.Exit(4)
	}
}
