package lifecycle

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type cache struct {
	layersDir   string
	buildpackID string
	layers      map[string]*LayerMetadata
}

func readCache(layersDir, buildpackID string) (cache, error) {
	layers := make(map[string]*LayerMetadata)
	tomls, err := filepath.Glob(filepath.Join(layersDir, buildpackID, "*.toml"))
	if err != nil {
		return cache{}, err
	}
	for _, toml := range tomls {
		name := strings.TrimRight(filepath.Base(toml), ".toml")
		layers[name], _ = readTOML(toml)
		if sha, err := ioutil.ReadFile(filepath.Join(layersDir, buildpackID, name+".sha")); err == nil {
			layers[name].SHA = string(sha)
		}
	}
	return cache{
		layersDir:   layersDir,
		buildpackID: buildpackID,
		layers:      layers,
	}, nil
}

type cacheType int

const (
	staleNoMetadata cacheType = iota
	staleWrongSHA
	nonLaunch // we can't determine whether the cache is stale for launch=false layers
	valid
)

func (c *cache) classifyLayer(layerName string, metadata *BuildpackMetadata) cacheType {
	cachedLayer, ok := c.layers[layerName]
	if !cachedLayer.Launch {
		return nonLaunch
	}
	if metadata == nil {
		return staleNoMetadata
	}
	layerMetadata, ok := metadata.Layers[layerName]
	if !ok {
		return staleNoMetadata
	}
	if layerMetadata.SHA != cachedLayer.SHA {
		return staleWrongSHA
	}
	return valid
}

func (c *cache) removeLayer(layerName string) error {
	if err := os.RemoveAll(c.layerPath(layerName)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(c.layerPath(layerName) + ".sha"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(c.layerPath(layerName) + ".toml"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (c *cache) layerPath(layerName string) string {
	return filepath.Join(c.layersDir, c.buildpackID, layerName)
}
