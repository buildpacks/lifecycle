package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/GoogleContainerTools/kaniko/pkg/config"
	cfg "github.com/redhat-buildpacks/poc/kaniko/buildpackconfig"
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

	// start with the base image given to us by the builder
	current_base_image := baseimage

	for _, d := range meta.Dockerfiles {
		// skip the extensions not of this type
		if d.Type != kind {
			continue
		}

		toMultiArg := func(args []DockerfileArg) []string {
			var result []string
			for _, arg := range args {
				result = append(result, fmt.Sprintf("%s=%s", arg.Key, arg.Value))
			}

			result = append(result, fmt.Sprintf(`base_image=%s`, current_base_image))
			return result
		}

		if kind == "build" {
			// TODO: execute all but the FROM clauses in the Dockerfile?
			// Define opts
			opts := config.KanikoOptions{
				BuildArgs:      toMultiArg(d.Args),
				DockerfilePath: d.Path,
				Cache:          true,
				// CacheOptions: config.CacheOptions{
				// 	CacheDir: b.CacheDir,
				// },
				IgnoreVarRun: true,
				NoPush:       true,
				SrcContext:   b.WorkspaceDir,
				SnapshotMode: "full",
				// OCILayoutPath:       "/layers/kaniko",
				// ImageNameDigestFile: fmt.Sprintf("/layers/kaniko/idk%s", b.Destination),
				RegistryOptions: config.RegistryOptions{
					SkipTLSVerify: true,
				},
				// IgnorePaths:  b.IgnorePaths,
				// TODO: we can not output a tar for intermediate images, we only need the last one
				// TarPath:      "/layers/kaniko/new_base.tar",
				// Destinations: []string{b.Destination},
				// ForceBuildMetadata: true,
				Cleanup: false,
			}

			// Build the Dockerfile
			fmt.Printf("Building the %s\n", opts.DockerfilePath)
			err := b.BuildDockerFile(opts)
			if err != nil {
				panic(err)
			}
			continue
		}

		// run extensions

		// we need a registry right now, because kaniko is pulling the image to build on top of for subsequent Dockerfile exts
		registryHost := os.Getenv("REGISTRY_HOST")
		b.Destination = fmt.Sprintf("%s/extended/runimage", registryHost)
		fmt.Printf("Destination Image: %s\n", b.Destination)

		// Define opts
		opts := config.KanikoOptions{
			BuildArgs:      toMultiArg(d.Args),
			DockerfilePath: d.Path,
			Cache:          true,
			// CacheOptions: config.CacheOptions{
			// 	CacheDir: b.CacheDir,
			// },
			IgnoreVarRun: true,
			// NoPush:       true,
			SrcContext:   b.WorkspaceDir,
			SnapshotMode: "full",
			// OCILayoutPath:       "/layers/kaniko",
			// ImageNameDigestFile: fmt.Sprintf("/layers/kaniko/idk%s", b.Destination),
			// IgnorePaths:  b.IgnorePaths,
			// TODO: we can not output a tar for intermediate images, we only need the last one
			// TarPath:      "/layers/kaniko/new_base.tar",
			Destinations: []string{b.Destination},
			// ForceBuildMetadata: true,
			Cleanup: true,
			RegistryOptions: config.RegistryOptions{
				SkipTLSVerify: true,
			},
		}

		// Build the Dockerfile
		fmt.Printf("Building the %s\n", opts.DockerfilePath)
		err := b.BuildDockerFile(opts)
		if err != nil {
			panic(err)
		}

		// TODO: next base image is the one we just built, we should use digest instead
		current_base_image = b.Destination
	}

	// run buildpacks now
	if kind == "build" {
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
}

func untar(tarPath, workingDirectory string) error {
	command := exec.Command("tar", "xvf", tarPath)
	command.Dir = workingDirectory
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func layerTarfiles(manifestPath string) ([]string, error) {
	// manifestPath := "/layers/kaniko/manifest.json"
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
