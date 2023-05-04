package platform

import (
	"path/filepath"
	"time"

	"github.com/buildpacks/lifecycle/internal/path"
)

const (
	DefaultAnalyzedFile        = "analyzed.toml"
	DefaultExtendKind          = "build"
	DefaultExtendedDir         = "extended"
	DefaultGeneratedDir        = "generated"
	DefaultGroupFile           = "group.toml"
	DefaultLogLevel            = "info"
	DefaultOrderFile           = "order.toml"
	DefaultPlanFile            = "plan.toml"
	DefaultPlatformAPI         = "0.3"
	DefaultProjectMetadataFile = "project-metadata.toml"
	DefaultReportFile          = "report.toml"
)

var (
	// CNBOrderPath is the default order path if the order file does not exist in the layers directory.
	CNBOrderPath               = filepath.Join(path.RootDir, "cnb", "order.toml")
	DefaultAppDir              = filepath.Join(path.RootDir, "workspace")
	DefaultBuildConfigDir      = filepath.Join(path.RootDir, "cnb", "build-config")
	DefaultBuildpacksDir       = filepath.Join(path.RootDir, "cnb", "buildpacks")
	DefaultBuildpacksioSBOMDir = filepath.Join(path.RootDir, "cnb", "lifecycle")
	DefaultExtensionsDir       = filepath.Join(path.RootDir, "cnb", "extensions")
	DefaultKanikoCacheTTL      = 14 * (24 * time.Hour)
	DefaultLauncherPath        = filepath.Join(path.RootDir, "cnb", "lifecycle", "launcher"+path.ExecExt)
	DefaultLayersDir           = filepath.Join(path.RootDir, "layers")
	DefaultPlatformDir         = filepath.Join(path.RootDir, "platform")
	DefaultRunPath             = filepath.Join(path.RootDir, "cnb", "run.toml")
	DefaultStackPath           = filepath.Join(path.RootDir, "cnb", "stack.toml")
)
