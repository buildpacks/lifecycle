package platform

import (
	"path/filepath"

	"github.com/buildpacks/lifecycle/api"
)

var (
	// TODO: when all phases use the "inputs resolver" pattern, these variables can be internal to the package
	PlaceholderAnalyzedPath        = filepath.Join("<layers>", DefaultAnalyzedFile)
	PlaceholderGroupPath           = filepath.Join("<layers>", DefaultGroupFile)
	PlaceholderOrderPath           = filepath.Join("<layers>", DefaultOrderFile)
	PlaceholderPlanPath            = filepath.Join("<layers>", DefaultPlanFile)
	PlaceholderProjectMetadataPath = filepath.Join("<layers>", DefaultProjectMetadataFile)
	PlaceholderReportPath          = filepath.Join("<layers>", DefaultReportFile)

	DefaultAppDir        = filepath.Join(rootDir, "workspace")
	DefaultBuildpacksDir = filepath.Join(rootDir, "cnb", "buildpacks")
	DefaultLauncherPath  = filepath.Join(rootDir, "cnb", "lifecycle", "launcher"+execExt)
	DefaultLayersDir     = filepath.Join(rootDir, "layers")
	DefaultPlatformDir   = filepath.Join(rootDir, "platform")
	DefaultStackPath     = filepath.Join(rootDir, "cnb", "stack.toml")
)

const (
	DefaultAnalyzedFile        = "analyzed.toml"
	DefaultGroupFile           = "group.toml"
	DefaultOrderFile           = "order.toml"
	DefaultPlanFile            = "plan.toml"
	DefaultProjectMetadataFile = "project-metadata.toml"
	DefaultReportFile          = "report.toml"
)

func defaultPath(placeholderPath, layersDir string, platformAPI *api.Version) string {
	filename := filepath.Base(placeholderPath)
	if (platformAPI).LessThan("0.5") || (layersDir == "") {
		// prior to platform api 0.5, the default directory was the working dir.
		// layersDir is unset when this call comes from the rebaser - will be fixed as part of https://github.com/buildpacks/spec/issues/156
		return filepath.Join(".", filename)
	}
	return filepath.Join(layersDir, filename)
}
