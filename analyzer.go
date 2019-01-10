package lifecycle

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/image"
)

type Analyzer struct {
	Buildpacks []*Buildpack
	AppDir     string
	LayersDir  string
	In         []byte
	Out, Err   *log.Logger
}

func (a *Analyzer) Analyze(image image.Image) error {
	found, err := image.Found()
	if err != nil {
		return err
	}

	var metadata AppImageMetadata
	if !found {
		a.Out.Printf("WARNING: image '%s' not found or requires authentication to access\n", image.Name())
	} else {
		metadata, err = a.getMetadata(image)
		if err != nil {
			return err
		}
	}
	return a.analyze(metadata)
}

func (a *Analyzer) analyze(metadata AppImageMetadata) error {
	groupBPs := a.buildpacks()

	err := a.removeOldBackpackLayersNotInGroup(groupBPs)
	if err != nil {
		return err
	}

	for buildpackID := range groupBPs {
		cache, err := readCache(a.LayersDir, buildpackID)
		if err != nil {
			return err
		}

		for cachedLayer := range cache.layers {
			cacheType := cache.classifyLayer(cachedLayer, bpMetadata(buildpackID, metadata))
			switch cacheType {
			case staleNoMetadata:
				a.Out.Printf("removing stale cached launch layer '%s/%s', not in metadata \n", buildpackID, cachedLayer)
				if err := cache.removeLayer(cachedLayer); err != nil {
					return err
				}
			case staleWrongSHA:
				a.Out.Printf("removing stale cached launch layer '%s/%s'", buildpackID, cachedLayer)
				if err := cache.removeLayer(cachedLayer); err != nil {
					return err
				}
			case nonLaunch:
				a.Out.Printf("using cached layer '%s/%s'", buildpackID, cachedLayer)
			case valid:
				a.Out.Printf("using cached launch layer '%s/%s'", buildpackID, cachedLayer)
			}
		}

		for layer, data := range a.layersToAnalyze(buildpackID, metadata) {
			if !data.Build {
				a.Out.Printf("writing metadata for layer '%s/%s'", buildpackID, layer)
				if err := writeTOML(filepath.Join(a.LayersDir, buildpackID, layer+".toml"), data); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (a *Analyzer) layersToAnalyze(buildpack string, metadata AppImageMetadata) map[string]LayerMetadata {
	layers := make(map[string]LayerMetadata)
	bpMetadata := bpMetadata(buildpack, metadata)
	if bpMetadata != nil {
		layers = bpMetadata.Layers
	}
	return layers
}

func bpMetadata(buildpackID string, metadata AppImageMetadata) *BuildpackMetadata {
	for _, buildpackMetaData := range metadata.Buildpacks {
		if buildpackMetaData.ID == buildpackID {
			return &buildpackMetaData
		}
	}

	return nil
}

func (a *Analyzer) getMetadata(image image.Image) (AppImageMetadata, error) {
	metadata := AppImageMetadata{}
	label, err := image.Label(MetadataLabel)
	if err != nil {
		return metadata, err
	}
	if label == "" {
		a.Out.Printf("WARNING: previous image '%s' does not have '%s' label", image.Name(), MetadataLabel)
		return metadata, nil
	}

	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		a.Out.Printf("WARNING: previous image '%s' has incompatible '%s' label\n", image.Name(), MetadataLabel)
		return metadata, nil
	}
	return metadata, nil
}

func (a *Analyzer) buildpacks() map[string]struct{} {
	buildpacks := make(map[string]struct{}, len(a.Buildpacks))
	for _, b := range a.Buildpacks {
		buildpacks[b.EscapedID()] = struct{}{}
	}
	return buildpacks
}

func (a *Analyzer) removeOldBackpackLayersNotInGroup(groupBPs map[string]struct{}) error {
	cachedBPs, err := a.cachedBuildpacks()
	if err != nil {
		return err
	}

	for _, cachedBP := range cachedBPs {
		_, exists := groupBPs[cachedBP]
		if !exists {
			a.Out.Printf("removing cached layers for buildpack '%s' not in group\n", cachedBP)
			if err := os.RemoveAll(filepath.Join(a.LayersDir, cachedBP)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (a *Analyzer) cachedBuildpacks() ([]string, error) {
	cachedBps := make([]string, 0, 0)
	bpDirs, err := filepath.Glob(filepath.Join(a.LayersDir, "*"))
	if err != nil {
		return nil, err
	}
	appDirInfo, err := os.Stat(a.AppDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "stat app dir")
	}
	for _, dir := range bpDirs {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if !os.SameFile(appDirInfo, info) && info.IsDir() {
			cachedBps = append(cachedBps, filepath.Base(dir))
		}
	}
	return cachedBps, nil
}

func writeTOML(path string, metadata LayerMetadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	fh, err := os.Create(path)
	if err != nil {
		return err
	}

	defer fh.Close()
	return toml.NewEncoder(fh).Encode(metadata)
}

func readTOML(path string) (*LayerMetadata, error) {
	var metadata LayerMetadata
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	_, err = toml.DecodeFile(path, &metadata)
	if err != nil {
		return nil, err
	}

	return &metadata, err
}
