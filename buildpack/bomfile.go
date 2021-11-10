package buildpack

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack/layertypes"
)

const (
	LayerTypeBuild LayerType = iota
	LayerTypeCache
	LayerTypeLaunch
)

const (
	mediaTypeCycloneDX   = "application/vnd.cyclonedx+json"
	mediaTypeSPDX        = "application/spdx+json"
	mediaTypeSyft        = "application/vnd.syft+json"
	mediaTypeUnsupported = "unsupported"
)

type LayerType int

type BOMFile struct {
	BuildpackID string
	LayerName   string
	LayerType   LayerType
	Path        string
}

// Name() returns the destination filename for a given BOM file
// cdx files should be renamed to "bom.cdx.json"
// spdx files should be renamed to "bom.spdx.json"
// syft files should be renamed to "bom.syft.json"
// If the BOM is neither cdx, spdx, nor syft, the 2nd return argument
// will return an error to indicate an unsupported format
func (b *BOMFile) Name() (string, error) {
	switch b.mediaType() {
	case mediaTypeCycloneDX:
		return "bom.cdx.json", nil
	case mediaTypeSPDX:
		return "bom.spdx.json", nil
	case mediaTypeSyft:
		return "bom.syft.json", nil
	default:
		return "", errors.Errorf("unsupported bom format: '%s'", b.Path)
	}
}

func (b *BOMFile) mediaType() string {
	name := filepath.Base(b.Path)

	switch {
	case strings.HasSuffix(name, "bom.cdx.json"):
		return mediaTypeCycloneDX
	case strings.HasSuffix(name, "bom.spdx.json"):
		return mediaTypeSPDX
	case strings.HasSuffix(name, "bom.syft.json"):
		return mediaTypeSyft
	default:
		return mediaTypeUnsupported
	}
}

func validateMediaTypes(bp GroupBuildpack, bomfiles []BOMFile, sbomMediaTypes []string) error {
	contains := func(vs []string, t string) bool {
		for _, v := range vs {
			if v == t {
				return true
			}
		}
		return false
	}

	for _, bomFile := range bomfiles {
		mediaType := bomFile.mediaType()
		switch mediaType {
		case mediaTypeUnsupported:
			return errors.Errorf("unsupported bom format: '%s'", bomFile.Path)
		default:
			if !contains(sbomMediaTypes, mediaType) {
				return errors.Errorf("sbom type '%s' not declared for buildpack: '%s'", mediaType, bp.String())
			}
		}
	}

	return nil
}

func processBOMFiles(layersDir string, bp GroupBuildpack, pathToLayerMetadataFile map[string]layertypes.LayerMetadataFile, sbomMediaTypes []string) ([]BOMFile, error) {
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

	return files, validateMediaTypes(bp, files, sbomMediaTypes)
}
