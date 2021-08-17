package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layer"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	dockertypes "github.com/docker/docker/api/types"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle/archive"
)

const (
	linuxBaseImage   = "gcr.io/distroless/static"
	windowsBaseImage = "mcr.microsoft.com/windows/nanoserver:1809-amd64"
)

// commandline flags
var (
	lifecyclePath        string      // path to lifecycle TGZ
	tags                 stringSlice // tag reference to write lifecycle image
	targetOS, targetArch string      // operating system and CPU architecture
	useDaemon            bool        // export to docker daemon
)

type stringSlice []string

func (s *stringSlice) String() string {
	return fmt.Sprintf("%+v", *s)
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// Creates lifecycle image from lifecycle tgz
func main() {
	flag.StringVar(&lifecyclePath, "lifecyclePath", "", "path to lifecycle TGZ")
	flag.StringVar(&targetOS, "os", runtime.GOOS, "operating system")
	flag.StringVar(&targetArch, "arch", runtime.GOARCH, "CPU architecture")
	flag.Var(&tags, "tag", "tag reference to write lifecycle image")
	flag.BoolVar(&useDaemon, "daemon", false, "export to docker daemon")

	flag.Parse()
	if lifecyclePath == "" || len(tags) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	baseImage := linuxBaseImage
	if targetOS == "windows" {
		baseImage = windowsBaseImage
	}

	var img imgutil.Image
	if useDaemon {
		dockerClient, err := dockercli.NewClientWithOpts(dockercli.FromEnv, dockercli.WithVersion("1.38"))
		if err != nil {
			log.Fatal("Failed to initialize docker client:", err)
		}
		info, err := dockerClient.Info(context.Background())
		if err != nil {
			log.Fatal("Failed to to get daemon info:", err)
		}
		if info.OSType != targetOS {
			log.Fatal("Target OS and daemon OS must match")
		}
		daemonArch := info.Architecture
		if daemonArch == "x86_64" {
			daemonArch = "amd64"
		}
		if daemonArch != targetArch {
			log.Fatal("Target architecture and daemon architecture must match")
		}
		err = pullImage(dockerClient, baseImage)
		if err != nil {
			log.Fatal("Failed to pull base image:", err)
		}
		img, err = local.NewImage(tags[0], dockerClient, local.FromBaseImage(baseImage))
		if err != nil {
			log.Fatal("Failed to create local image:", err)
		}
	} else {
		var err error
		img, err = remote.NewImage(tags[0], authn.DefaultKeychain, remote.FromBaseImage(baseImage), remote.WithDefaultPlatform(imgutil.Platform{
			Architecture: targetArch,
			OS:           targetOS,
		}))
		if err != nil {
			log.Fatal("Failed to create remote image:", err)
		}
	}

	layerPath := lifecycleLayer()
	if err := img.AddLayer(layerPath); err != nil {
		log.Fatal("Failed to add layer:", err)
	}
	defer os.Remove(layerPath)
	descriptor := readDescriptor()
	if err := img.SetLabel("io.buildpacks.lifecycle.apis", apisLabel(descriptor)); err != nil {
		log.Fatal("Failed to set 'io.buildpacks.lifecycle.apis' label:", err)
	}
	if err := img.SetLabel("io.buildpacks.lifecycle.version", descriptor.Lifecycle.Version); err != nil {
		log.Fatal("Failed to set 'io.buildpacks.lifecycle.version' label:", err)
	}
	if err := img.SetLabel("io.buildpacks.builder.metadata", legacyLabel(descriptor)); err != nil {
		log.Fatal("Failed to set 'io.buildpacks.builder.metadata' label:", err)
	}
	workDir := "/layers"
	if targetOS == "windows" {
		workDir = `c:\layers`
	}
	if err := img.SetWorkingDir(workDir); err != nil {
		log.Fatal("Failed to set working directory:", err)
	}
	if len(tags) > 1 {
		if err := img.Save(tags[1:]...); err != nil {
			log.Fatal("Failed to save image:", err)
		}
	} else {
		if err := img.Save(); err != nil {
			log.Fatal("Failed to save image:", err)
		}
	}
	id, err := img.Identifier()
	if err != nil {
		log.Fatal("Failed to get image id:", err)
	}
	switch v := id.(type) {
	case local.IDIdentifier:
		fmt.Println("Image ID:", v.String())
	case remote.DigestIdentifier:
		fmt.Println("Digest:", v.Digest.DigestStr())
	default:
	}
}

type Descriptor struct {
	APIs      APIs          `toml:"apis"`
	Lifecycle LifecycleInfo `toml:"lifecycle"`
}

type APIs struct {
	Buildpack APISet `toml:"buildpack" json:"buildpack"`
	Platform  APISet `toml:"platform" json:"platform"`
}

type LifecycleInfo struct {
	Version string `toml:"version" json:"version"`
}

type APISet struct {
	Deprecated []string `toml:"deprecated" json:"deprecated"`
	Supported  []string `toml:"supported" json:"supported"`
}

func readDescriptor() Descriptor {
	descriptor := Descriptor{}
	f, err := os.Open(lifecyclePath)
	if err != nil {
		log.Fatalf("Failed to open -lifecyclePath %s: %s", lifecyclePath, err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		log.Fatalf("Failed to create gzip reader from lifecyle at path %s: %s", lifecyclePath, err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			log.Fatalf("Failed to read descriptor from lifecycle tgz at path '%s': %s", lifecyclePath, err)
		}
		if filepath.Base(hdr.Name) != "lifecycle.toml" {
			continue
		}
		_, err = toml.DecodeReader(tr, &descriptor)
		if err != nil {
			log.Fatalf("Failed to read descriptor from lifecycle tgz at path '%s': %s", lifecyclePath, err)
		}
		break
	}
	return descriptor
}

type BuilderLabel struct {
	Lifecycle LifecycleInfo  `json:"lifecycle"`
	API       BuildLabelAPIs `json:"api"`
}

type BuildLabelAPIs struct {
	Buildpack string `json:"buildpack"`
	Platform  string `json:"platform"`
}

func legacyLabel(descriptor Descriptor) string {
	label := BuilderLabel{
		Lifecycle: descriptor.Lifecycle,
		API: BuildLabelAPIs{
			Buildpack: descriptor.APIs.Buildpack.Supported[0],
			Platform:  descriptor.APIs.Platform.Supported[0],
		},
	}
	labelContents, err := json.Marshal(label)
	if err != nil {
		log.Fatal("Failed to marshal builder label", err)
	}
	return string(labelContents)
}

func apisLabel(descriptor Descriptor) string {
	labelContents, err := json.Marshal(descriptor.APIs)
	if err != nil {
		log.Fatal("Failed to marshal apis label", err)
	}
	return string(labelContents)
}

func lifecycleLayer() string {
	f, err := os.Open(lifecyclePath)
	if err != nil {
		log.Fatalf("Failed to open -lifecyclePath %s: %s", lifecyclePath, err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		log.Fatalf("Failed to create gzip reader from lifecyle at path %s: %s", lifecyclePath, err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	ntr := archive.NewNormalizingTarReader(tr)
	ntr.PrependDir("/cnb/")

	lf, err := ioutil.TempFile("", "lifecycle-layer")
	if err != nil {
		log.Fatal("Failed to create temp layer file", err)
	}
	defer lf.Close()

	var ntw *archive.NormalizingTarWriter
	var mode int64
	if targetOS == "windows" {
		ntw = archive.NewNormalizingTarWriter(layer.NewWindowsWriter(lf))
		mode = 0777
	} else {
		ntw = archive.NewNormalizingTarWriter(tar.NewWriter(lf))
		mode = 0755
	}

	ntw.WithModTime(archive.NormalizedModTime)
	if targetOS == "windows" {
		ntw.WithUID(1) //gets translated to user permissions in windows writer
		ntw.WithGID(1) //gets translated to user permissions in windows writer
	} else {
		ntw.WithUID(0)
		ntw.WithGID(0)
	}
	if err := ntw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "/cnb",
		Mode:     mode,
	}); err != nil {
		log.Fatalf("WriteHeader(/cnb): %v", err)
	}
	for {
		hdr, err := ntr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("Error reading tar header:", err)
		}
		if err := ntw.WriteHeader(hdr); err != nil {
			log.Fatal("Error writing tar header:", err)
		}
		if hdr.Typeflag == tar.TypeReg {
			_, err := io.Copy(ntw, ntr)
			if err != nil {
				log.Fatalf("Error writing contents for entry '%s': %s", hdr.Name, err)
			}
		}
	}
	if err := ntw.Close(); err != nil {
		log.Fatal("Error closing tar writer:", err)
	}
	return lf.Name()
}

func pullImage(dockerCli dockercli.CommonAPIClient, ref string) error {
	rc, err := dockerCli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{})
	if err != nil {
		// Retry
		rc, err = dockerCli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{})
		if err != nil {
			return err
		}
	}
	defer rc.Close()
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return nil
}
