package files

import (
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/log"
)

// Run is provided by the platform as run.toml to record information about the run images
// that may be used during export.
// Data from the selected run image is serialized by the exporter as the `runImage` key in the `io.buildpacks.lifecycle.metadata` label
// on the output image for use during rebase.
type Run struct {
	Images []RunImageForExport `json:"-" toml:"images"`
}

func ReadRun(runPath string, logger log.Logger) (Run, error) {
	var runMD Run
	if _, err := toml.DecodeFile(runPath, &runMD); err != nil {
		if os.IsNotExist(err) {
			logger.Infof("no run metadata found at path '%s'\n", runPath)
			return Run{}, nil
		}
		return Run{}, err
	}
	return runMD, nil
}
