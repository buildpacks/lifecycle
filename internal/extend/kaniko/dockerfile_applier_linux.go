//go:build linux

package kaniko

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/GoogleContainerTools/kaniko/pkg/executor"
	"github.com/GoogleContainerTools/kaniko/pkg/image"
	"github.com/buildpacks/imgutil/layout"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"

	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/internal/selective"
)

func (a *DockerfileApplier) Apply(workspace string, digestToExtend string, dockerfiles []extend.Dockerfile, options extend.Options) error {
	// Configure kaniko
	baseImage, err := readOCI(ociPrefix + filepath.Join(kanikoBaseCacheDir, digestToExtend))
	if err != nil {
		return fmt.Errorf("getting base image for digest '%s': %w", digestToExtend, err)
	}
	image.RetrieveRemoteImage = func(image string, opts config.RegistryOptions, customPlatform string) (v1.Image, error) {
		return baseImage, nil // force kaniko to return this base image, instead of trying to pull it from a registry
	}

	origTopLayer, err := topLayer(baseImage)
	if err != nil {
		return fmt.Errorf("getting top layer: %w", err)
	}

	// Apply each Dockerfile in order, updating the base image for the next Dockerfile to be the intermediate extended image
	baseImageRef := fmt.Sprintf("base@%s", digestToExtend)
	for idx, dfile := range dockerfiles {
		// TODO: prefix special build args
		opts := createOptions(workspace, baseImageRef, dfile, options)

		// Change to root directory; kaniko does this here:
		// https://github.com/GoogleContainerTools/kaniko/blob/09e70e44d9e9a3fecfcf70cb809a654445837631/cmd/executor/cmd/root.go#L140-L142
		if err := os.Chdir("/"); err != nil {
			return err
		}

		// Apply Dockerfile
		var err error
		a.logger.Debugf("Applying the Dockerfile at %s...", dfile.Path)
		baseImage, err = executor.DoBuild(&opts)
		if err != nil {
			return fmt.Errorf("applying dockerfile: %w", err)
		}

		// Update base image to point to intermediate image
		intermediateImageHash, err := baseImage.Digest()
		if err != nil {
			return fmt.Errorf("getting hash for intermediate image %d: %w", idx+1, err)
		}
		baseImageRef = "intermediate-extended@" + intermediateImageHash.String()
	}

	if err = a.copySelective(baseImage, origTopLayer); err != nil {
		return fmt.Errorf("copying selective image to output directory: %w", err)
	}

	if err = setImageEnvVarsInCurrentContext(baseImage); err != nil {
		return fmt.Errorf("setting environment variables from image in current context: %w", err)
	}

	return cleanKanikoDir()
}

func readOCI(digestRef string) (v1.Image, error) {
	if !strings.HasPrefix(digestRef, "oci:") {
		return nil, fmt.Errorf("expected '%s' to have prefix 'oci:'", digestRef)
	}
	path, err := layout.FromPath(strings.TrimPrefix(digestRef, "oci:"))
	if err != nil {
		return nil, fmt.Errorf("getting layout from path: %w", err)
	}
	hash, err := v1.NewHash(filepath.Base(digestRef))
	if err != nil {
		return nil, fmt.Errorf("getting hash from reference '%s': %w", digestRef, err)
	}
	v1Image, err := path.Image(hash) // // FIXME: we may want to implement path.Image(h) in the imgutil 'sparse' package so that trying to access layers on this image errors with a helpful message
	if err != nil {
		return nil, fmt.Errorf("getting image from hash '%s': %w", hash.String(), err)
	}
	return v1Image, nil
}

func topLayer(image v1.Image) (string, error) {
	manifest, err := image.Manifest()
	if err != nil {
		return "", fmt.Errorf("getting image manifest: %w", err)
	}
	layers := manifest.Layers
	if len(layers) == 0 {
		return "", nil
	}
	layer := layers[len(layers)-1]
	return layer.Digest.String(), nil
}

func (a *DockerfileApplier) copySelective(image v1.Image, origTopLayerHash string) error {
	// save sparse image (manifest and config)
	imageHash, err := image.Digest()
	if err != nil {
		return fmt.Errorf("getting image hash: %w", err)
	}
	outputPath := filepath.Join(a.outputDir, imageHash.String())
	layoutPath, err := selective.Write(outputPath, empty.Index) // FIXME: this should use the imgutil layout/sparse package instead, but for some reason sparse.NewImage().Save() fails when the provided base image is already sparse
	if err != nil {
		return fmt.Errorf("initializing selective image: %w", err)
	}
	if err = layoutPath.AppendImage(image); err != nil {
		return fmt.Errorf("saving selective image: %w", err)
	}
	// get all image layers (we will only copy those following the original top layer)
	layers, err := image.Layers()
	if err != nil {
		return fmt.Errorf("getting image layers: %w", err)
	}
	var (
		currentHash  v1.Hash
		needsCopying bool
	)
	if origTopLayerHash == "" { // if the original base image had no layers, copy all the layers
		needsCopying = true
	}
	for _, currentLayer := range layers {
		currentHash, err = currentLayer.Digest()
		if err != nil {
			return fmt.Errorf("getting layer hash: %w", err)
		}
		switch {
		case needsCopying:
			// TODO: do in a go func
			if err = a.copyLayer(currentLayer, outputPath); err != nil {
				return fmt.Errorf("copying layer: %w", err)
			}
		case currentHash.String() == origTopLayerHash:
			needsCopying = true
			continue
		default:
			continue
		}
	}
	return nil
}

func (a *DockerfileApplier) copyLayer(layer v1.Layer, toSparseImage string) error {
	digest, err := layer.Digest()
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(toSparseImage, "blobs", digest.Algorithm, digest.Hex))
	defer f.Close()
	if err != nil {
		return err
	}
	rc, err := layer.Compressed() // TODO: if exporting to a daemon, this should be uncompressed
	defer rc.Close()
	if err != nil {
		return err
	}
	_, err = io.Copy(f, rc)
	return err
}

func setImageEnvVarsInCurrentContext(image v1.Image) error {
	extendedConfig, err := image.ConfigFile()
	if err != nil {
		return fmt.Errorf("getting config for extended image: %w", err)
	}
	for _, env := range extendedConfig.Config.Env {
		parts := strings.Split(env, "=")
		if len(parts) != 2 {
			return fmt.Errorf("parsing env '%s': expected format 'key=value'", env)
		}
		if err := os.Setenv(parts[0], parts[1]); err != nil {
			return fmt.Errorf("setting env: %w", err)
		}
	}
	return nil
}

func cleanKanikoDir() error {
	fis, err := os.ReadDir(kanikoDir)
	if err != nil {
		return fmt.Errorf("reading kaniko dir '%s': %w", kanikoDir, err)
	}
	for _, fi := range fis {
		if fi.Name() == "cache" {
			continue
		}
		toRemove := filepath.Join(kanikoDir, fi.Name())
		if err = os.RemoveAll(toRemove); err != nil {
			return fmt.Errorf("removing directory item '%s': %w", toRemove, err)
		}
	}
	return nil
}
