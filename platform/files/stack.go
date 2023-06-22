package files

import (
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/log"
)

// Stack (deprecated as of Platform API 0.12) is provided by the platform as stack.toml to record information about the run images
// that may be used during export.
// It is also serialized by the exporter as the `stack` key in the `io.buildpacks.lifecycle.metadata` label on the output image
// for use during rebase.
// The location of the file can be specified by providing `-stack <path>` to the lifecycle.
type Stack struct {
	RunImage RunImageForExport `json:"runImage" toml:"run-image"`
}

func (s *Stack) ToRunImageForRebase() *RunImageForRebase {
	return &RunImageForRebase{
		Image:   s.RunImage.Image,
		Mirrors: s.RunImage.Mirrors,
	}
}

type RunImageForExport struct {
	Image   string   `toml:"image" json:"image"`
	Mirrors []string `toml:"mirrors" json:"mirrors,omitempty"`
}

func ReadStack(stackPath string, logger log.Logger) (Stack, error) {
	var stackMD Stack
	if _, err := toml.DecodeFile(stackPath, &stackMD); err != nil {
		if os.IsNotExist(err) {
			logger.Infof("no stack metadata found at path '%s'\n", stackPath)
			return Stack{}, nil
		}
		return Stack{}, err
	}
	return stackMD, nil
}
