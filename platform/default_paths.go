package platform

import (
	"os"
	"path/filepath"

	"github.com/buildpacks/lifecycle/api"
)

const (
	DefaultAnalyzedFile = "analyzed.toml"
	DefaultGroupFile    = "group.toml"
	DefaultOrderFile    = "order.toml"
	DefaultPlanFile     = "plan.toml"
	// TODO: future work should move project metadata and report to this file
)

var (
	PlaceholderAnalyzedPath = filepath.Join("<layers>", DefaultAnalyzedFile)
	PlaceholderGroupPath    = filepath.Join("<layers>", DefaultGroupFile)
	PlaceholderOrderPath    = filepath.Join("<layers>", DefaultOrderFile)
	PlaceholderPlanPath     = filepath.Join("<layers>", DefaultPlanFile)
)

func defaultPath(placeholderPath, layersDir string, platformAPI *api.Version) string {
	if placeholderPath == PlaceholderOrderPath {
		return defaultOrderPath(layersDir, platformAPI)
	}

	filename := filepath.Base(placeholderPath)
	if (platformAPI).LessThan("0.5") || (layersDir == "") {
		// prior to platform api 0.5, the default directory was the working dir.
		// layersDir is unset when this call comes from the rebaser - will be fixed as part of https://github.com/buildpacks/spec/issues/156
		return filepath.Join(".", filename)
	}
	return filepath.Join(layersDir, filename)
}

func defaultOrderPath(layersDir string, platformAPI *api.Version) string {
	cnbOrderPath := filepath.Join(rootDir, "cnb", "order.toml")
	if platformAPI.LessThan("0.6") {
		return cnbOrderPath
	}

	layersOrderPath := filepath.Join(layersDir, "order.toml")
	if _, err := os.Stat(layersOrderPath); err != nil {
		return cnbOrderPath
	}
	return layersOrderPath
}
