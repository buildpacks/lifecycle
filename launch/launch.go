package launch

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type Process struct {
	Type             string         `toml:"type" json:"type"`
	Command          []string       `toml:"-" json:"-"` // ignored
	RawCommandValue  toml.Primitive `toml:"command" json:"command"`
	Args             []string       `toml:"args" json:"args"`
	Direct           bool           `toml:"direct" json:"direct"`
	Default          bool           `toml:"default,omitempty" json:"default,omitempty"`
	BuildpackID      string         `toml:"buildpack-id" json:"buildpackID"`
	WorkingDirectory string         `toml:"working-dir,omitempty" json:"working-dir,omitempty"`
}

func (p Process) NoDefault() Process {
	p.Default = false
	return p
}

// ProcessPath returns the absolute path to the symlink for a given process type
func ProcessPath(pType string) string {
	return filepath.Join(ProcessDir, pType+exe)
}

type Metadata struct {
	Processes  []Process   `toml:"processes" json:"processes"`
	Buildpacks []Buildpack `toml:"buildpacks" json:"buildpacks"`
}

// DecodeLaunchMetadataTOML reads a launch.toml file
func DecodeLaunchMetadataTOML(path string, launchmd *Metadata) error {
	// decode the common bits
	md, err := toml.DecodeFile(path, &launchmd)
	if err != nil {
		return err
	}

	if err = DecodeProcesses(launchmd.Processes, md); err != nil {
		return err
	}

	return nil
}

func DecodeProcesses(processes []Process, md toml.MetaData) error {
	// decode the process.commands, which will differ based on APIs
	// processes are defined differently depending on API version
	// and will be decoded into different values
	for i, process := range processes {
		var commandString string
		if err := md.PrimitiveDecode(process.RawCommandValue, &commandString); err == nil {
			processes[i].Command = []string{commandString}
			continue
		}

		var command []string
		if err := md.PrimitiveDecode(process.RawCommandValue, &command); err != nil {
			return err
		}
		processes[i].Command = command
	}

	return nil
}

// Matches is used by goMock to compare two Metadata objects in tests
// when matching expected calls to methods containing Metadata objects
func (m Metadata) Matches(x interface{}) bool {
	metadatax, ok := x.(Metadata)
	if !ok {
		return false
	}

	// don't compare Processes directly, we will compare those individually next
	if s := cmp.Diff(metadatax, m, cmpopts.IgnoreFields(Metadata{}, "Processes")); s != "" {
		return false
	}

	// we need to ignore the RawCommandValue field because it is a toml.Primitive and is not part of our equality
	for i, p := range m.Processes {
		if s := cmp.Diff(metadatax.Processes[i], p, cmpopts.IgnoreFields(Process{}, "RawCommandValue")); s != "" {
			return false
		}
	}

	return true
}

func (m Metadata) String() string {
	return fmt.Sprintf("%+v %+v", m.Processes, m.Buildpacks)
}

func (m Metadata) FindProcessType(pType string) (Process, bool) {
	for _, p := range m.Processes {
		if p.Type == pType {
			return p, true
		}
	}
	return Process{}, false
}

type Buildpack struct {
	API string `toml:"api"`
	ID  string `toml:"id"`
}

func EscapeID(id string) string {
	return strings.ReplaceAll(id, "/", "_")
}

func GetMetadataFilePath(layersDir string) string {
	return path.Join(layersDir, "config", "metadata.toml")
}
