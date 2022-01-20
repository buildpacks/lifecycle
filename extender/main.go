package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/GoogleContainerTools/kaniko/pkg/config"
	cfg "github.com/redhat-buildpacks/poc/kaniko/buildpackconfig"
	"github.com/redhat-buildpacks/poc/kaniko/util"
)

type BuildMetadata struct {
	Dockerfiles []Dockerfile `toml:"dockerfiles,omitempty" json:"dockerfiles,omitempty"`
}

type DockerfileArg struct {
	Key   string `toml:"name"`
	Value string `toml:"value"`
}

type Dockerfile struct {
	ExtensionID string          `toml:"extension_id"`
	Path        string          `toml:"path"`
	Type        string          `toml:"type"`
	Args        []DockerfileArg `toml:"args"`
}

func main() {
	mode := "kaniko"
	kind := "build"
	baseimage := "alpine"

	if len(os.Args) > 3 {
		mode = os.Args[1]
		kind = os.Args[2]
		baseimage = os.Args[3]
	}

	fmt.Printf("Starting Extender with '%s' mode ...\n", mode)

	switch mode {
	case "kaniko":
		doKaniko(kind, baseimage)
	//case "buildah":
	//	doBuildah()
	default:
		panic(fmt.Sprintf("Unsupported mode provided: '%s'", mode))
	}
}

// TODO: this only knows how to extend the build image (in container). Extension for run image will be added later.
func doKaniko(kind, baseimage string) {
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

	// Read metadata toml
	meta := BuildMetadata{}
	_, err := toml.DecodeFile(filepath.Join("/layers/config/metadata.toml"), &meta)
	if err != nil {
		panic(err)
	}

	toMultiArg := func(args []DockerfileArg) []string {
		var result []string
		for _, arg := range args {
			result = append(result, fmt.Sprintf("%s=%s", arg.Key, arg.Value))
		}

		result = append(result, fmt.Sprintf(`base_image=%s`, baseimage))
		return result
	}

	for _, d := range meta.Dockerfiles {
		// Define opts
		opts := config.KanikoOptions{
			BuildArgs:          toMultiArg(d.Args),
			DockerfilePath:     d.Path,
			CacheOptions:       config.CacheOptions{CacheDir: b.CacheDir},
			IgnoreVarRun:       true,
			NoPush:             true,
			SrcContext:         b.WorkspaceDir,
			SnapshotMode:       "full",
			IgnorePaths:        b.IgnorePaths,
			TarPath:            b.LayerTarFileName,
			Destinations:       []string{b.Destination},
			ForceBuildMetadata: true,
		}

		// Build the Dockerfile
		fmt.Printf("Building the %s\n", opts.DockerfilePath)
		err := b.BuildDockerFile(opts)
		if err != nil {
			panic(err)
		}
	}

	// Log the content of the Kaniko dir
	fmt.Printf("Reading dir content of: %s\n", b.KanikoDir)
	util.ReadFilesFromPath(b.KanikoDir)

	// TODO: caching doesn't seem to be working at the moment. Need to investigate...
	// Copy the tgz layer file to the Cache dir
	srcPath := path.Join("/", b.LayerTarFileName)
	dstPath := path.Join(b.CacheDir, b.LayerTarFileName)

	// Ensure cache directory exists
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

	if kind == "build" {
		fmt.Printf("Extract the content of the tarball file %s under the cache %s\n", b.Opts.TarPath, b.Opts.CacheDir)
		err = untar(dstPath, b.CacheDir)
		if err != nil {
			panic(err)
		}

		layerFiles, err := layerTarfiles()
		if err != nil {
			panic(err)
		}

		// We're in "build" mode, untar layers to root filesystem: /
		for _, layerFile := range layerFiles {
			workingDirectory := "/"
			tarPath := "/layers/kaniko/" + layerFile

			err = untar(tarPath, workingDirectory)
			if err != nil {
				panic(err)
			}
		}

		// Run the build for buildpacks with lowered privileges.
		// We must assume that this extender is run as root.
		cmd := exec.Command("/cnb/lifecycle/builder", "-app", "/workspace", "-log-level", "debug")
		cmd.Env = append(cmd.Env, "CNB_PLATFORM_API=0.8")
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: 1000, Gid: 1000}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			panic(err)
		}
	}

	if kind == "run" {
		// We're in "run" mode, leave the layer tar files for exporter to collect later.
		// TODO: do we do anything else? Or is kaniko snapshot tar output all we need?
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
