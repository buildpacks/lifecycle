package platform

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
)

var (
	DefaultAnalyzedFile        = "analyzed.toml"
	DefaultGroupFile           = "group.toml"
	DefaultOrderFile           = "order.toml"
	DefaultPlanFile            = "plan.toml"
	DefaultProjectMetadataFile = "project-metadata.toml"
	DefaultReportFile          = "report.toml"

	PlaceholderAnalyzedPath        = filepath.Join("<layers>", DefaultAnalyzedFile)
	PlaceholderGroupPath           = filepath.Join("<layers>", DefaultGroupFile)
	PlaceholderPlanPath            = filepath.Join("<layers>", DefaultPlanFile)
	PlaceholderProjectMetadataPath = filepath.Join("<layers>", DefaultProjectMetadataFile)
	PlaceholderReportPath          = filepath.Join("<layers>", DefaultReportFile)
	PlaceholderOrderPath           = filepath.Join("<layers>", DefaultOrderFile)
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

func readStack(stackPath string, logger lifecycle.Logger) (platform.StackMetadata, error) {
	var stackMD platform.StackMetadata
	if _, err := toml.DecodeFile(stackPath, &stackMD); err != nil {
		if os.IsNotExist(err) {
			logger.Infof("no stack metadata found at path '%s'\n", stackPath)
		} else {
			return platform.StackMetadata{}, err
		}
	}
	return stackMD, nil
}
