package lifecycle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/uuid"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/internal/selective"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/log"
)

type Extender struct {
	AppDir       string // explicitly ignored by the Dockerfile applier, also the Dockefile build context
	ExtendedDir  string // output directory for extended image layers
	GeneratedDir string // input Dockerfiles are found here
	ImageRef     string // the image to extend
	LayersDir    string // explicitly ignored by the Dockerfile applier
	PlatformDir  string // explicitly ignored by the Dockerfile applier

	CacheTTL          time.Duration            // a platform input
	DockerfileApplier DockerfileApplier        // uses kaniko, BuildKit, or other to apply the provided Dockerfile to the provided image
	Extensions        []buildpack.GroupElement // extensions are ordered from group.toml
}

//go:generate mockgen -package testmock -destination testmock/dockerfile_applier.go github.com/buildpacks/lifecycle DockerfileApplier
type DockerfileApplier interface {
	ImageFor(reference string) (v1.Image, error)
	Apply(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger log.Logger) (v1.Image, error)
	Cleanup() error
}

type ExtenderFactory struct {
	apiVerifier   BuildpackAPIVerifier
	configHandler ConfigHandler
}

func NewExtenderFactory(apiVerifier BuildpackAPIVerifier, configHandler ConfigHandler) *ExtenderFactory {
	return &ExtenderFactory{
		apiVerifier:   apiVerifier,
		configHandler: configHandler,
	}
}

func (f *ExtenderFactory) NewExtender(
	analyzedPath string,
	appDir string,
	extendedDir string,
	generatedDir string,
	groupPath string,
	layersDir string,
	platformDir string,
	cacheTTL time.Duration,
	dockerfileApplier DockerfileApplier,
	logger log.Logger,
) (*Extender, error) {
	extender := &Extender{
		AppDir:            appDir,
		ExtendedDir:       extendedDir,
		GeneratedDir:      generatedDir,
		LayersDir:         layersDir,
		PlatformDir:       platformDir,
		CacheTTL:          cacheTTL,
		DockerfileApplier: dockerfileApplier,
	}
	if err := f.setImageRef(extender, analyzedPath); err != nil {
		return nil, err
	}
	if err := f.setExtensions(extender, groupPath, logger); err != nil {
		return nil, err
	}
	return extender, nil
}

func (f *ExtenderFactory) setImageRef(extender *Extender, path string) error {
	analyzedMD, err := f.configHandler.ReadAnalyzed(path)
	if err != nil {
		return err
	}
	if analyzedMD.BuildImage != nil {
		extender.ImageRef = analyzedMD.BuildImage.Reference
	}
	return nil
}

func (f *ExtenderFactory) setExtensions(extender *Extender, path string, logger log.Logger) error {
	_, groupExt, err := f.configHandler.ReadGroup(path)
	if err != nil {
		return fmt.Errorf("reading group: %w", err)
	}
	for i := range groupExt {
		groupExt[i].Extension = true
	}
	if err = f.verifyAPIs(groupExt, logger); err != nil {
		return err
	}
	extender.Extensions = groupExt
	return nil
}

func (f *ExtenderFactory) verifyAPIs(groupExt []buildpack.GroupElement, logger log.Logger) error {
	for _, groupEl := range groupExt {
		if err := f.apiVerifier.VerifyBuildpackAPI(groupEl.Kind(), groupEl.String(), groupEl.API, logger); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extender) Extend(kind string, logger log.Logger) error {
	switch kind {
	case buildpack.DockerfileKindBuild:
		return e.extendBuild(logger)
	case buildpack.DockerfileKindRun:
		return e.extendRun(logger)
	default:
		return nil
	}
}

func (e *Extender) extendBuild(logger log.Logger) error {
	origBaseImage, err := e.DockerfileApplier.ImageFor(e.ImageRef)
	if err != nil {
		return fmt.Errorf("getting build image to extend: %w", err)
	}

	extendedImage, err := e.extend(buildpack.DockerfileKindBuild, origBaseImage, logger)
	if err != nil {
		return fmt.Errorf("extending build image: %w", err)
	}

	if err = setImageEnvVarsInCurrentContext(extendedImage); err != nil {
		return fmt.Errorf("setting environment variables from extended image in current context: %w", err)
	}

	return e.DockerfileApplier.Cleanup()
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

func (e *Extender) extendRun(logger log.Logger) error {
	origBaseImage, err := e.DockerfileApplier.ImageFor(e.ImageRef)
	if err != nil {
		return fmt.Errorf("getting run image to extend: %w", err)
	}

	origTopLayer, err := topLayer(origBaseImage)
	if err != nil {
		return fmt.Errorf("getting original run image top layer: %w", err)
	}

	extendedImage, err := e.extend(buildpack.DockerfileKindRun, origBaseImage, logger)
	if err != nil {
		return fmt.Errorf("extending run image: %w", err)
	}

	if err = e.saveSelective(extendedImage, origTopLayer); err != nil {
		return fmt.Errorf("copying selective image to output directory: %w", err)
	}

	return e.DockerfileApplier.Cleanup()
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

func (e *Extender) saveSelective(image v1.Image, origTopLayerHash string) error {
	// save sparse image (manifest and config)
	imageHash, err := image.Digest()
	if err != nil {
		return fmt.Errorf("getting image hash: %w", err)
	}
	outputPath := filepath.Join(e.ExtendedDir, imageHash.String())
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
			if err = copyLayer(currentLayer, outputPath); err != nil {
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

func copyLayer(layer v1.Layer, toSparseImage string) error {
	digest, err := layer.Digest()
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(toSparseImage, "blobs", digest.Algorithm, digest.Hex))
	// defer f.Close() // TODO: why is this a compile error in GoLand?
	if err != nil {
		return err
	}
	rc, err := layer.Compressed() // TODO: if exporting to a daemon, this should be uncompressed
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(f, rc)
	return err
}

func (e *Extender) extend(kind string, baseImage v1.Image, logger log.Logger) (v1.Image, error) {
	logger.Debugf("Extending base image for %s: %s", kind, e.ImageRef)
	dockerfiles, err := e.dockerfilesFor(kind, logger)
	if err != nil {
		return nil, fmt.Errorf("getting %s dockerfiles: %w", kind, err)
	}

	buildOptions := e.extendOptions()

	var userID, groupID int // TODO: read these from the baseImage
	for _, dockerfile := range dockerfiles {
		dockerfile.Args = append([]extend.Arg{
			{Name: "build_id", Value: uuid.New().String()}, // TODO: make constants
			{Name: "user_id", Value: fmt.Sprintf("%d", userID)},
			{Name: "group_id", Value: fmt.Sprintf("%d", groupID)},
		}, dockerfile.Args...)
		if baseImage, err = e.DockerfileApplier.Apply(
			dockerfile,
			baseImage,
			buildOptions,
			logger,
		); err != nil {
			return nil, fmt.Errorf("applying dockerfile to image: %w", err)
		}
		// TODO: update userID, groupID
	}
	return baseImage, nil
}

func (e *Extender) dockerfilesFor(kind string, logger log.Logger) ([]extend.Dockerfile, error) {
	var dockerfiles []extend.Dockerfile
	for _, ext := range e.Extensions {
		dockerfile, err := e.dockerfileFor(kind, ext.ID)
		if err != nil {
			return nil, err
		}
		if dockerfile != nil {
			logger.Debugf("Found %s Dockerfile for extension '%s'", kind, ext.ID)
			dockerfiles = append(dockerfiles, *dockerfile)
		}
	}
	return dockerfiles, nil
}

func (e *Extender) dockerfileFor(kind, extID string) (*extend.Dockerfile, error) {
	dockerfilePath := filepath.Join(e.GeneratedDir, kind, launch.EscapeID(extID), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, nil
	}

	configPath := filepath.Join(e.GeneratedDir, kind, launch.EscapeID(extID), "extend-config.toml")
	var config buildpack.ExtendConfig
	_, err := toml.DecodeFile(configPath, &config)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	var args []extend.Arg
	for _, arg := range config.Build.Args {
		args = append(args, toExtendArg(arg))
	}

	return &extend.Dockerfile{
		Path: dockerfilePath,
		Args: args,
	}, nil
}

func toExtendArg(arg buildpack.ExtendArg) extend.Arg {
	return extend.Arg{Name: arg.Name, Value: arg.Value}
}

func (e *Extender) extendOptions() extend.Options {
	return extend.Options{
		BuildContext: e.AppDir,
		CacheTTL:     e.CacheTTL,
		IgnorePaths:  []string{e.AppDir, e.LayersDir, e.PlatformDir},
	}
}
