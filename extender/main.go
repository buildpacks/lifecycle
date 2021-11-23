package main

import (
	"encoding/json"
	"fmt"
	cfg "github.com/redhat-buildpacks/poc/kaniko/buildpackconfig"
	"github.com/redhat-buildpacks/poc/kaniko/util"
	"os"
	"os/exec"
	"path"
	"strings"
)

func main() {
	mode := "kaniko"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	fmt.Printf("Starting Extender with '%s' mode ...\n", mode)

	switch mode {
	case "kaniko":
		doKaniko()
	//case "buildah":
	//	doBuildah()
	default:
		panic(fmt.Sprintf("Unsupported mode provided: '%s'", mode))
	}
}

func doKaniko() {
	fmt.Println("Starting the Kaniko application to process a Dockerfile ...")

	// Create a buildPackConfig and set the default values
	fmt.Println("Initialize the BuildPackConfig and set the defaults values ...")
	b := cfg.NewBuildPackConfig()
	b.InitDefaults()
	b.ExtractLayers = false

	fmt.Printf("Kaniko      dir: %s\n", b.KanikoDir)
	fmt.Printf("Workspace   dir: %s\n", b.WorkspaceDir)
	fmt.Printf("Cache       dir: %s\n", b.CacheDir)
	fmt.Printf("Dockerfile name: %s\n", b.DockerFileName)
	fmt.Printf("Extract layer files ? %T\n", b.ExtractLayers)

	// Build the Dockerfile
	fmt.Printf("Building the %s\n", b.DockerFileName)
	err := b.BuildDockerFile()
	if err != nil {
		panic(err)
	}
	// Log the content of the Kaniko dir
	fmt.Printf("Reading dir content of: %s\n", b.KanikoDir)
	util.ReadFilesFromPath(b.KanikoDir)

	// Copy the tgz layer file to the Cache dir
	srcPath := path.Join("/", b.LayerTarFileName)
	dstPath := path.Join(b.CacheDir, b.LayerTarFileName)

	fmt.Printf("Creating %s dir ...\n", b.CacheDir)
	err = os.MkdirAll(b.CacheDir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Copy the %s file to the %s dir ...\n", srcPath, dstPath)
	err = util.File(srcPath, dstPath)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Extract the content of the tarball file %s under the cache %s\n", b.Opts.TarPath, b.Opts.CacheDir)
	// ANTHONY: everything below just isn't working...
	// b.ExtractImageTarFile(dstPath)

	// 	fmt.Println("Extract the layer file(s)")
	//	descriptor, err := b.LoadDescriptorAndConfig()
	//	if err != nil {
	//		panic(err)
	//	}
	//
	//	fmt.Printf("%+v\n", descriptor)
	//	layers := descriptor[0].Layers
	//	b.ExtractTarGZFilesWithoutBaseImage(layers[0])

	err = untar(dstPath, b.CacheDir)
	if err != nil {
		panic(err)
	}

	layerFiles, err := layerTarfiles()
	if err != nil {
		panic(err)
	}

	for _, layerFile := range layerFiles {
		workingDirectory := "/layers/kaniko/" + strings.TrimSuffix(layerFile, ".tar.gz")
		tarPath := "/layers/kaniko/" + layerFile

		err := os.MkdirAll(workingDirectory, os.ModePerm)
		if err != nil {
			panic(err)
		}

		err = untar(tarPath, workingDirectory)
		if err != nil {
			panic(err)
		}
	}
}

func untar(tarPath, workingDirectory string) error {
	command := exec.Command("tar", "xvf", tarPath)
	command.Dir = workingDirectory
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func layerTarfiles() ([]string, error) {
	manifestPath := "/layers/kaniko/manifest.json"
	var manifest []struct {
		Layers []string `json:"Layers"`
	}

	f, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	defer f.Close()

	err = json.NewDecoder(f).Decode(&manifest)
	if err != nil {
		return nil, err
	}

	if len(manifest) == 0 || len(manifest[0].Layers) == 0 {
		return nil, nil
	}

	// Return layers except first one (the base layer)
	return manifest[0].Layers[1:], nil
}
