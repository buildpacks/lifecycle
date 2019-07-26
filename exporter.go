package lifecycle

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/local"
	"github.com/buildpack/imgutil/remote"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/metadata"
)

type Exporter struct {
	Buildpacks   []*Buildpack
	ArtifactsDir string
	In           []byte
	Out, Err     *log.Logger
	UID, GID     int
}

func (e *Exporter) Export(
	layersDir,
	appDir string,
	workingImage imgutil.Image,
	origMetadata metadata.AppImageMetadata,
	additionalNames []string,
	launcher string,
	stack metadata.StackMetadata,
) error {
	var err error

	meta := metadata.AppImageMetadata{}

	meta.RunImage.TopLayer, err = workingImage.TopLayer()
	if err != nil {
		return errors.Wrap(err, "get run image top layer SHA")
	}

	identifier, err := workingImage.Identifier()
	if err != nil {
		return errors.Wrap(err, "get run image id or digest")
	}

	meta.RunImage.Reference = identifier.String()
	meta.Stack = stack

	meta.App.SHA, err = e.addOrReuseLayer(workingImage, &layer{path: appDir, identifier: "app"}, origMetadata.App.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting app layer")
	}

	meta.Config.SHA, err = e.addOrReuseLayer(workingImage, &layer{path: filepath.Join(layersDir, "config"), identifier: "config"}, origMetadata.Config.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting config layer")
	}

	meta.Launcher.SHA, err = e.addOrReuseLayer(workingImage, &layer{path: launcher, identifier: "launcher"}, origMetadata.Launcher.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting launcher layer")
	}

	for _, bp := range e.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, *bp)
		if err != nil {
			return errors.Wrapf(err, "reading layers for buildpack '%s'", bp.ID)
		}
		bpMD := metadata.BuildpackMetadata{ID: bp.ID, Version: bp.Version, Layers: map[string]metadata.LayerMetadata{}}

		for _, layer := range bpDir.findLayers(launch) {
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

				e.Out.Printf("Reusing layer '%s' with SHA %s\n", layer.Identifier(), origLayerMetadata.SHA)
				if err := workingImage.ReuseLayer(origLayerMetadata.SHA); err != nil {
					return errors.Wrapf(err, "reusing layer: '%s'", layer.Identifier())
				}
				lmd.SHA = origLayerMetadata.SHA
			}
			bpMD.Layers[layer.name()] = lmd
		}

		if malformedLayers := bpDir.findLayers(malformed); len(malformedLayers) > 0 {
			ids := make([]string, 0, len(malformedLayers))
			for _, ml := range malformedLayers {
				ids = append(ids, ml.Identifier())
			}
			return fmt.Errorf("failed to parse metadata for layers '%s'", ids)
		}

		meta.Buildpacks = append(meta.Buildpacks, bpMD)
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}

	if err = workingImage.SetLabel(metadata.AppMetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}

	buildMD := &BuildMetadata{}
	if _, err := toml.DecodeFile(metadata.MetadataFilePath(layersDir), buildMD); err != nil {
		return errors.Wrap(err, "read build metadata")
	} else {
		buildJson, err := json.Marshal(metadata.BuildMetadata{BOM: buildMD.BOM})
		if err != nil {
			return errors.Wrap(err, "parse build metadata")
		}

		if err = workingImage.SetLabel(metadata.BuildMetadataLabel, string(buildJson)); err != nil {
			return errors.Wrap(err, "set build image metadata label")
		}
	}

	if err = workingImage.SetEnv(cmd.EnvLayersDir, layersDir); err != nil {
		return errors.Wrapf(err, "set app image env %s", cmd.EnvLayersDir)
	}

	if err = workingImage.SetEnv(cmd.EnvAppDir, appDir); err != nil {
		return errors.Wrapf(err, "set app image env %s", cmd.EnvAppDir)
	}

	if err = workingImage.SetEntrypoint(launcher); err != nil {
		return errors.Wrap(err, "setting entrypoint")
	}

	if err = workingImage.SetCmd(); err != nil { // Note: Command intentionally empty
		return errors.Wrap(err, "setting cmd")
	}

	return e.saveImage(workingImage, additionalNames)
}

func (e *Exporter) addOrReuseLayer(image imgutil.Image, layer identifiableLayer, previousSha string) (string, error) {
	tarPath := filepath.Join(e.ArtifactsDir, escapeIdentifier(layer.Identifier())+".tar")
	sha, err := archive.WriteTarFile(layer.Path(), tarPath, e.UID, e.GID)
	if err != nil {
		return "", errors.Wrapf(err, "exporting layer '%s'", layer.Identifier())
	}
	if sha == previousSha {
		e.Out.Printf("Reusing layer '%s' with SHA %s\n", layer.Identifier(), sha)
		return sha, image.ReuseLayer(previousSha)
	}
	e.Out.Printf("Exporting layer '%s' with SHA %s\n", layer.Identifier(), sha)
	return sha, image.AddLayer(tarPath)
}

func (e *Exporter) saveImage(image imgutil.Image, additionalNames []string) error {
	var saveErr error
	if err := image.Save(additionalNames...); err != nil {
		var ok bool
		if saveErr, ok = err.(imgutil.SaveError); !ok {
			return errors.Wrap(err, "saving image")
		}
	}

	e.Out.Println("*** Images:")
	for _, n := range append([]string{image.Name()}, additionalNames...) {
		e.Out.Printf("      %s - %s\n", n, getSaveStatus(saveErr, n))
	}

	id, idErr := image.Identifier()
	if idErr != nil {
		if saveErr != nil {
			return &MultiError{Errors: []error{idErr, saveErr}}
		}
		return idErr
	}

	e.logReference(id)
	return saveErr
}

func (e *Exporter) logReference(identifier imgutil.Identifier) {
	switch v := identifier.(type) {
	case local.IDIdentifier:
		e.Out.Printf("\n*** Image ID: %s\n", v.String())
	case remote.DigestIdentifier:
		e.Out.Printf("\n*** Digest: %s\n", v.Digest.DigestStr())
	default:
		e.Out.Printf("\n*** Reference: %s\n", v.String())
	}
}

type MultiError struct {
	Errors []error
}

func (me *MultiError) Error() string {
	return fmt.Sprintf("failed with multiple errors %+v", me.Errors)
}

func getSaveStatus(err error, imageName string) string {
	if err != nil {
		if saveErr, ok := err.(imgutil.SaveError); ok {
			for _, d := range saveErr.Errors {
				if d.ImageName == imageName {
					return d.Cause.Error()
				}
			}
		}
	}
	return "succeeded"
}
