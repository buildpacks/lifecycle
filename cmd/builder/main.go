package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/sclevine/lifecycle"
)

var (
	groupPath    string
	metadataPath string
)

func init() {
	flag.StringVar(&groupPath, "group", "", "buildpack group file path (JSON)")
	flag.StringVar(&metadataPath, "metadata", "", "metadata output file path (JSON)")
}

func main() {
	flag.Parse()
	groupFile, err := os.Open(groupPath)
	if err != nil {
		log.Fatalln("Failed to open buildpack group file:", err)
	}
	defer groupFile.Close()
	var group lifecycle.BuildpackGroup
	if err := json.NewDecoder(groupFile).Decode(&group); err != nil {
		log.Fatalln("Failed to read buildpack group:", err)
	}
	builder := &lifecycle.Builder{
		PlatformDir: lifecycle.DefaultPlatformDir,
		Buildpacks:  group,
		Out:         os.Stdout,
		Err:         os.Stderr,
	}
	env := &lifecycle.POSIXEnv{
		Getenv:  os.Getenv,
		Setenv:  os.Setenv,
		Environ: os.Environ,
	}
	metadata, err := builder.Build(
		lifecycle.DefaultAppDir,
		lifecycle.DefaultLaunchDir,
		lifecycle.DefaultCacheDir,
		env,
	)
	if err != nil {
		log.Fatalln("Failed to build:", err)
	}
	mdFile, err := os.Create(metadataPath)
	if err != nil {
		log.Fatalln("Failed to create metadata file:", err)
	}
	defer mdFile.Close()
	if err := json.NewEncoder(mdFile).Encode(metadata); err != nil {
		log.Fatalln("Failed to write metadata:", err)
	}
}
