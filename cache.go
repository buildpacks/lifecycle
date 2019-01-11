package lifecycle

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type cache struct {
	buildpackDir string
	layers       []string
}

func readCache(layersDir, buildpackID string) (cache, error) {
	buildpackDir := filepath.Join(layersDir, buildpackID)
	tomls, err := filepath.Glob(filepath.Join(buildpackDir, "*.toml"))
	if err != nil {
		return cache{}, err
	}
	layers := make([]string, 0, len(tomls))
	for _, toml := range tomls {
		name := strings.TrimRight(filepath.Base(toml), ".toml")
		layers = append(layers, name)
	}
	return cache{
		buildpackDir: buildpackDir,
		layers:       layers,
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
	cachedLayer, err := c.readLayer(layerName)
	if err != nil {
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

func (c *cache) readLayer(layerName string) (*LayerMetadata, error) {
	var metadata LayerMetadata
	tomlPath := c.layerPath(layerName) + ".toml"
	fh, err := os.Open(tomlPath)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	_, err = toml.DecodeFile(tomlPath, &metadata)
	if err != nil {
		return nil, err
	}
	if sha, err := ioutil.ReadFile(c.layerPath(layerName) + ".sha"); err == nil {
		metadata.SHA = string(sha)
	}
	return &metadata, err
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
	return filepath.Join(c.buildpackDir, layerName)
}

func (c *cache) writeMetadata(layerName string, metadataLayers map[string]LayerMetadata) error {
	layerMetadata := metadataLayers[layerName]
	path := filepath.Join(c.layerPath(layerName) + ".toml")
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
