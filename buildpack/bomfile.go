package buildpack

import (
	"path/filepath"
	"strings"

	"github.com/buildpacks/lifecycle/buildpack/layertypes"
)

const (
	LayerTypeBuild LayerType = iota
	LayerTypeCache
	LayerTypeLaunch
)

type LayerType int

type BOMFile struct {
	BuildpackID string
	LayerName   string
	LayerType   LayerType
	Path        string
}

func (b *BOMFile) Name() (string, bool) {
	name := filepath.Base(b.Path)

	switch {
	case strings.HasSuffix(name, "bom.cdx.json"):
		return "bom.cdx.json", true
	case strings.HasSuffix(name, "bom.spdx.json"):
		return "bom.spdx.json", true
	default:
		return "", false
	}
}

func processBOMFiles(layersDir string, bp GroupBuildpack, pathToLayerMetadataFile map[string]layertypes.LayerMetadataFile, logger Logger) ([]BOMFile, error) {
	var (
		layerGlob = filepath.Join(layersDir, "*.bom.*.json")
		files     []BOMFile
	)

	matches, err := filepath.Glob(layerGlob)
	if err != nil {
		return nil, err
	}

	for _, m := range matches {
		layerDir, file := filepath.Split(m)
		layerName := strings.SplitN(file, ".", 2)[0]

		if layerName == "launch" {
			files = append(files, BOMFile{
				BuildpackID: bp.ID,
				LayerType:   LayerTypeLaunch,
				Path:        m,
			})

			continue
		}

		if layerName == "build" {
			files = append(files, BOMFile{
				BuildpackID: bp.ID,
				LayerType:   LayerTypeBuild,
				Path:        m,
			})

			continue
		}

		meta, ok := pathToLayerMetadataFile[filepath.Join(layerDir, layerName)]
		if !ok {
			continue
		}

		if meta.Launch {
			files = append(files, BOMFile{
				BuildpackID: bp.ID,
				LayerName:   layerName,
				LayerType:   LayerTypeLaunch,
				Path:        m,
			})
		} else {
			files = append(files, BOMFile{
				BuildpackID: bp.ID,
				LayerName:   layerName,
				LayerType:   LayerTypeBuild,
				Path:        m,
			})
		}

		if meta.Cache {
			files = append(files, BOMFile{
				BuildpackID: bp.ID,
				LayerName:   layerName,
				LayerType:   LayerTypeCache,
				Path:        m,
			})
		}
	}

	return files, nil
}
