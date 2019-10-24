package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/cache"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/metadata"
)

//go:generate mockgen -package testmock -destination testmock/cache.go github.com/buildpack/lifecycle Cache
type Cache interface {
	Name() string
	SetMetadata(metadata cache.Metadata) error
	RetrieveMetadata() (cache.Metadata, error)
	AddLayerFile(sha string, tarPath string) error
	ReuseLayer(sha string) error
	RetrieveLayer(sha string) (io.ReadCloser, error)
	Commit() error
}

type Exporter struct {
	Buildpacks   []Buildpack
	ArtifactsDir string
	Logger       Logger
	UID, GID     int
	tarHashes    map[string]string // Stores hashes of layer tarballs for reuse between the export and cache steps.
}

type LauncherConfig struct {
	Path     string
	Metadata metadata.LauncherMetadata
}

func (e *Exporter) Export(
	layersDir,
	appDir string,
	workingImage imgutil.Image,
	runImageRef string,
	origMetadata metadata.LayersMetadata,
	additionalNames []string,
	launcherConfig LauncherConfig,
	stack metadata.StackMetadata,
) error {
	var err error

	meta := metadata.LayersMetadata{}
	meta.RunImage.TopLayer, err = workingImage.TopLayer()
	if err != nil {
		return errors.Wrap(err, "get run image top layer SHA")
	}

	// TODO why are we saving stack in addition to runImageRef when the two can be different?
	meta.RunImage.Reference = runImageRef
	meta.Stack = stack

	meta.App.SHA, err = e.addOrReuseLayer(workingImage, &layer{path: appDir, identifier: "app"}, origMetadata.App.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting app layer")
	}

	meta.Config.SHA, err = e.addOrReuseLayer(workingImage, &layer{path: filepath.Join(layersDir, "config"), identifier: "config"}, origMetadata.Config.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting config layer")
	}

	meta.Launcher.SHA, err = e.addOrReuseLayer(workingImage, &layer{path: launcherConfig.Path, identifier: "launcher"}, origMetadata.Launcher.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting launcher layer")
	}

	for _, bp := range e.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, bp)
		if err != nil {
			return errors.Wrapf(err, "reading layers for buildpack '%s'", bp.ID)
		}

		bpMD := metadata.BuildpackLayersMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers:  map[string]metadata.BuildpackLayerMetadata{},
		}
		for _, layer := range bpDir.findLayers(launch) {
			layer := layer
			lmd, err := layer.read()
			if err != nil {
				return errors.Wrapf(err, "reading '%s' metadata", layer.Identifier())
			}

			if layer.hasLocalContents() {
				origLayerMetadata := origMetadata.MetadataForBuildpack(bp.ID).Layers[layer.name()]
				lmd.SHA, err = e.addOrReuseLayer(workingImage, &layer, origLayerMetadata.SHA)
				if err != nil {
					return err
				}
			} else {
				if lmd.Cache {
					return fmt.Errorf("layer '%s' is cache=true but has no contents", layer.Identifier())
				}
				origLayerMetadata, ok := origMetadata.MetadataForBuildpack(bp.ID).Layers[layer.name()]
				if !ok {
					return fmt.Errorf("cannot reuse '%s', previous image has no metadata for layer '%s'", layer.Identifier(), layer.Identifier())
				}

				e.Logger.Infof("Reusing layer '%s'\n", layer.Identifier())
				e.Logger.Debugf("Layer '%s' SHA: %s\n", layer.Identifier(), origLayerMetadata.SHA)
				if err := workingImage.ReuseLayer(origLayerMetadata.SHA); err != nil {
					return errors.Wrapf(err, "reusing layer: '%s'", layer.Identifier())
				}
				lmd.SHA = origLayerMetadata.SHA
			}
			bpMD.Layers[layer.name()] = lmd
		}
		meta.Buildpacks = append(meta.Buildpacks, bpMD)

		if malformedLayers := bpDir.findLayers(malformed); len(malformedLayers) > 0 {
			ids := make([]string, 0, len(malformedLayers))
			for _, ml := range malformedLayers {
				ids = append(ids, ml.Identifier())
			}
			return fmt.Errorf("failed to parse metadata for layers '%s'", ids)
		}
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}

	if err = workingImage.SetLabel(metadata.LayerMetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}

	buildMD := &BuildMetadata{}
	if _, err := toml.DecodeFile(metadata.FilePath(layersDir), buildMD); err != nil {
		return errors.Wrap(err, "read build metadata")
	}

	if err := e.addBuildMetadataLabel(workingImage, buildMD.BOM, launcherConfig.Metadata); err != nil {
		return errors.Wrapf(err, "add build metadata label")
	}

	if err = workingImage.SetEnv(cmd.EnvLayersDir, layersDir); err != nil {
		return errors.Wrapf(err, "set app image env %s", cmd.EnvLayersDir)
	}

	if err = workingImage.SetEnv(cmd.EnvAppDir, appDir); err != nil {
		return errors.Wrapf(err, "set app image env %s", cmd.EnvAppDir)
	}

	if err = workingImage.SetEntrypoint(launcherConfig.Path); err != nil {
		return errors.Wrap(err, "setting entrypoint")
	}

	if err = workingImage.SetCmd(); err != nil { // Note: Command intentionally empty
		return errors.Wrap(err, "setting cmd")
	}

	return saveImage(workingImage, additionalNames, e.Logger)
}

func (e *Exporter) Cache(layersDir string, cacheStore Cache) error {
	var err error
	origMeta, err := cacheStore.RetrieveMetadata()
	if err != nil {
		return errors.Wrap(err, "metadata for previous cache")
	}
	meta := cache.Metadata{}

	for _, bp := range e.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, bp)
		if err != nil {
			return errors.Wrapf(err, "reading layers for buildpack '%s'", bp.ID)
		}

		bpMD := metadata.BuildpackLayersMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers:  map[string]metadata.BuildpackLayerMetadata{},
		}
		for _, layer := range bpDir.findLayers(cached) {
			layer := layer
			if !layer.hasLocalContents() {
				return fmt.Errorf("failed to cache layer '%s' because it has no contents", layer.Identifier())
			}
			lmd, err := layer.read()
			if err != nil {
				return errors.Wrapf(err, "reading %q metadata", layer.Identifier())
			}
			origLayerMetadata := origMeta.MetadataForBuildpack(bp.ID).Layers[layer.name()]
			if lmd.SHA, err = e.addOrReuseCacheLayer(cacheStore, &layer, origLayerMetadata.SHA); err != nil {
				return err
			}
			bpMD.Layers[layer.name()] = lmd
		}
		meta.Buildpacks = append(meta.Buildpacks, bpMD)
	}

	if err := cacheStore.SetMetadata(meta); err != nil {
		return errors.Wrap(err, "setting cache metadata")
	}
	if err := cacheStore.Commit(); err != nil {
		return errors.Wrap(err, "committing cache")
	}

	return nil
}

func (e *Exporter) tarLayer(layer identifiableLayer) (string, string, error) {
	tarPath := filepath.Join(e.ArtifactsDir, escapeID(layer.Identifier())+".tar")
	if e.tarHashes == nil {
		e.tarHashes = make(map[string]string)
	}
	if sha, ok := e.tarHashes[tarPath]; ok {
		e.Logger.Debugf("Reusing tarball for layer %q with SHA: %s\n", layer.Identifier(), sha)
		return tarPath, sha, nil
	}
	e.Logger.Debugf("Writing tarball for layer %q\n", layer.Identifier())
	sha, err := archive.WriteTarFile(layer.Path(), tarPath, e.UID, e.GID)
	if err != nil {
		return "", "", err
	}
	e.tarHashes[tarPath] = sha
	return tarPath, sha, nil
}

func (e *Exporter) addOrReuseLayer(image imgutil.Image, layer identifiableLayer, previousSHA string) (string, error) {
	tarPath, sha, err := e.tarLayer(layer)
	if err != nil {
		return "", errors.Wrapf(err, "tarring layer '%s'", layer.Identifier())
	}
	if sha == previousSHA {
		e.Logger.Infof("Reusing layer '%s'\n", layer.Identifier())
		e.Logger.Debugf("Layer '%s' SHA: %s\n", layer.Identifier(), sha)
		return sha, image.ReuseLayer(previousSHA)
	}
	e.Logger.Infof("Adding layer '%s'\n", layer.Identifier())
	e.Logger.Debugf("Layer '%s' SHA: %s\n", layer.Identifier(), sha)
	return sha, image.AddLayer(tarPath)
}

func (e *Exporter) addOrReuseCacheLayer(cache Cache, layer identifiableLayer, previousSHA string) (string, error) {
	tarPath, sha, err := e.tarLayer(layer)
	if err != nil {
		return "", errors.Wrapf(err, "tarring layer %q", layer.Identifier())
	}
	if sha == previousSHA {
		e.Logger.Infof("Reusing cache layer '%s'\n", layer.Identifier())
		e.Logger.Debugf("Layer '%s' SHA: %s\n", layer.Identifier(), sha)
		return sha, cache.ReuseLayer(previousSHA)
	}
	e.Logger.Infof("Adding cache layer '%s'\n", layer.Identifier())
	e.Logger.Debugf("Layer '%s' SHA: %s\n", layer.Identifier(), sha)
	return sha, cache.AddLayerFile(sha, tarPath)
}

func (e *Exporter) addBuildMetadataLabel(image imgutil.Image, plan []BOMEntry, launcherMD metadata.LauncherMetadata) error {
	var bps []metadata.BuildpackMetadata
	for _, bp := range e.Buildpacks {
		bps = append(bps, metadata.BuildpackMetadata{
			ID:      bp.ID,
			Version: bp.Version,
		})
	}

	buildJSON, err := json.Marshal(metadata.BuildMetadata{
		BOM:        plan,
		Buildpacks: bps,
		Launcher:   launcherMD,
	})
	if err != nil {
		return errors.Wrap(err, "parse build metadata")
	}

	if err := image.SetLabel(metadata.BuildMetadataLabel, string(buildJSON)); err != nil {
		return errors.Wrap(err, "set build image metadata label")
	}

	return nil
}
