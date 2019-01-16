package lifecycle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type bpLayersDir struct {
	path   string
	layers []bpLayer
	name   string
}

func readBuildpackLayersDir(layersDir, buildpackID string) (bpLayersDir, error) {
	path := filepath.Join(layersDir, buildpackID)
	tomls, err := filepath.Glob(filepath.Join(path, "*.toml"))
	if err != nil {
		return bpLayersDir{}, err
	}
	layers := make([]bpLayer, 0, len(tomls))
	for _, toml := range tomls {
		name := strings.TrimRight(filepath.Base(toml), ".toml")
		layers = append(layers, bpLayer{
			layer{
				path:       filepath.Join(layersDir, buildpackID, name),
				identifier: fmt.Sprintf("%s/%s", buildpackID, name),
			},
		})
	}
	return bpLayersDir{
		name:   buildpackID,
		path:   path,
		layers: layers,
	}, nil
}

func launch(md LayerMetadata) bool {
	return md.Launch
}

func nonCached(md LayerMetadata) bool {
	return !md.Cache
}

func (bd *bpLayersDir) findLayers(f func(layer LayerMetadata) bool) []bpLayer {
	var launchLayers []bpLayer
	for _, l := range bd.layers {
		md, err := l.read()
		if err == nil && f(md) {
			launchLayers = append(launchLayers, l)
		}
	}
	return launchLayers
}

func (bd *bpLayersDir) newBPLayer(name string) *bpLayer {
	return &bpLayer{
		layer{
			path:       filepath.Join(bd.path, name),
			identifier: fmt.Sprintf("%s/%s", bd.name, name),
		},
	}
}

type cacheType int

const (
	cacheStaleNoMetadata cacheType = iota
	cacheStaleWrongSHA
	cacheNotForLaunch // we can't determine whether the cache is stale for launch=false layers
	cacheValid
	cacheMalformed
)

type bpLayer struct {
	layer
}

func (bp *bpLayer) classifyCache(metadataLayers map[string]LayerMetadata) cacheType {
	cachedLayer, err := bp.read()
	if err != nil {
		return cacheMalformed
	}
	if !cachedLayer.Launch {
		return cacheNotForLaunch
	}
	if metadataLayers == nil {
		return cacheStaleNoMetadata
	}
	layerMetadata, ok := metadataLayers[bp.name()]
	if !ok {
		return cacheStaleNoMetadata
	}
	if layerMetadata.SHA != cachedLayer.SHA {
		return cacheStaleWrongSHA
	}
	return cacheValid
}

func (bp *bpLayer) read() (LayerMetadata, error) {
	var metadata LayerMetadata
	tomlPath := bp.path + ".toml"
	fh, err := os.Open(tomlPath)
	if err != nil {
		return LayerMetadata{}, err
	}
	defer fh.Close()
	_, err = toml.DecodeFile(tomlPath, &metadata)
	if err != nil {
		return LayerMetadata{}, err
	}
	if sha, err := ioutil.ReadFile(bp.path + ".sha"); err == nil {
		metadata.SHA = string(sha)
	}
	return metadata, err
}

func (bp *bpLayer) remove() error {
	if err := os.RemoveAll(bp.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(bp.path + ".sha"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(bp.path + ".toml"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (bp *bpLayer) writeMetadata(metadataLayers map[string]LayerMetadata) error {
	layerMetadata := metadataLayers[bp.name()]
	path := filepath.Join(bp.path + ".toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	return toml.NewEncoder(fh).Encode(layerMetadata)
}

func (bp *bpLayer) hasLocalContents() bool {
	_, err := ioutil.ReadDir(bp.path)

	return !os.IsNotExist(err)
}

func (bp *bpLayer) writeSha(sha string) error {
	if err := ioutil.WriteFile(filepath.Join(bp.path+".sha"), []byte(sha), 0755); err != nil {
		return err
	}
	return nil
}

func (bp *bpLayer) name() string {
	return filepath.Base(bp.path)
}

type layer struct {
	path       string
	identifier string
}

func (l *layer) Identifier() string {
	return l.identifier
}

func (l *layer) Path() string {
	return l.path
}

type identifiableLayer interface {
	Identifier() string
	Path() string
}
