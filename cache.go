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
	layers := map[string]*LayerMetadata{}
	tomls, err := filepath.Glob(filepath.Join(layersDir, buildpackID, "*.toml"))
	if err != nil {
		return cache{}, err
	}
	for _, toml := range tomls {
		name := strings.TrimRight(filepath.Base(toml), ".toml")
		layers[name], err = readTOML(toml)
		if err != nil {
			continue
		}
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
	cacheStaleNoMetadata cacheType = iota
	cacheStaleWrongSHA
	cacheNotForLaunch // we can't determine whether the cache is stale for launch=false layers
	cacheValid
	cacheMalformed
)

func (c *cache) classifyLayer(layerName string, metadataLayers map[string]LayerMetadata) cacheType {
	cachedLayer, ok := c.layers[layerName]
	if cachedLayer == nil {
		return cacheMalformed
	}
	if !cachedLayer.Launch {
		return cacheNotForLaunch
	}
	if metadataLayers == nil {
		return cacheStaleNoMetadata
	}
	layerMetadata, ok := metadataLayers[layerName]
	if !ok {
		return cacheStaleNoMetadata
	}
	if layerMetadata.SHA != cachedLayer.SHA {
		return cacheStaleWrongSHA
	}
	return cacheValid
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

func (c *cache) writeMetadata(layerName string, metadataLayers map[string]LayerMetadata) error {
	layerMetadata := metadataLayers[layerName]
	if err := writeTOML(filepath.Join(c.layerPath(layerName)+".toml"), layerMetadata); err != nil {
		return err
	}

	return nil
}
