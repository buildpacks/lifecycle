package launch

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/internal/encoding"

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

// processSerializer is used to encode a process to toml.
type processSerializer struct {
	Type             string   `toml:"type" json:"type"`
	Command          string   `toml:"command" json:"command"` // command is string
	Args             []string `toml:"args" json:"args"`
	Direct           bool     `toml:"direct" json:"direct"`
	Default          bool     `toml:"default,omitempty" json:"default,omitempty"`
	BuildpackID      string   `toml:"buildpack-id" json:"buildpackID"`
	WorkingDirectory string   `toml:"working-dir,omitempty" json:"working-dir,omitempty"`
}

// MarshalText implements the toml TextMarshaler interface to allow us more control when writing a Process to a toml file.
func (p Process) MarshalText() ([]byte, error) {
	serializer := processSerializer{
		Type:             p.Type,
		Command:          p.Command[0],
		Args:             append(p.Command[1:], p.Args[0:]...),
		Direct:           p.Direct,
		Default:          p.Default,
		BuildpackID:      p.BuildpackID,
		WorkingDirectory: p.WorkingDirectory,
	}
	bytes, err := encoding.MarshalTOML(&struct {
		*processSerializer
	}{
		processSerializer: &serializer,
	})
	return bytes, err
}

// UnmarshalTOML implements the toml Unmarshaler interface to allow us more control when reading a Process from toml.
func (p *Process) UnmarshalTOML(data interface{}) error {
	var tomlString string
	switch v := data.(type) {
	case string:
		tomlString = v
	case map[string]interface{}:
		// turn back into a string
		bytes, _ := encoding.MarshalTOML(v)
		tomlString = string(bytes)
	default:
		return errors.New("could not cast data to string")
	}

	// This is the same as launch.Process and exists to allow us to toml.Decode inside of UnmarshalTOML
	type pProcess struct {
		Type             string         `toml:"type" json:"type"`
		Command          []string       `toml:"-" json:"-"` // ignored
		RawCommandValue  toml.Primitive `toml:"command" json:"command"`
		Args             []string       `toml:"args" json:"args"`
		Direct           bool           `toml:"direct" json:"direct"`
		Default          bool           `toml:"default,omitempty" json:"default,omitempty"`
		BuildpackID      string         `toml:"buildpack-id" json:"buildpackID"`
		WorkingDirectory string         `toml:"working-dir,omitempty" json:"working-dir,omitempty"`
	}

	// unmarshal the common bits
	newProcess := pProcess{}
	md, err := toml.Decode(tomlString, &newProcess)
	if err != nil {
		return err
	}

	// handle the process.command, which will differ based on APIs
	var commandWasString bool
	var commandString string
	if err := md.PrimitiveDecode(newProcess.RawCommandValue, &commandString); err == nil {
		commandWasString = true
		newProcess.Command = []string{commandString}
	}

	if !commandWasString {
		var command []string
		if err := md.PrimitiveDecode(newProcess.RawCommandValue, &command); err != nil {
			return err
		}
		newProcess.Command = command
	}

	*p = Process(newProcess)
	return nil
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
	_, err := toml.DecodeFile(path, &launchmd)
	if err != nil {
		return err
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
