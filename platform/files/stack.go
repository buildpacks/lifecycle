package files

import (
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/internal/name"
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

type RunImageForExport struct {
	Image   string   `toml:"image,omitempty" json:"image,omitempty"`
	Mirrors []string `toml:"mirrors,omitempty" json:"mirrors,omitempty"`
}

// Contains returns true if the provided reference matches either the primary image,
// or the image mirrors.
func (r *RunImageForExport) Contains(ref string) bool {
	ref = name.ParseMaybe(ref)
	if name.ParseMaybe(r.Image) == ref {
		return true
	}
	for _, m := range r.Mirrors {
		if name.ParseMaybe(m) == ref {
			return true
		}
	}
	return false
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
