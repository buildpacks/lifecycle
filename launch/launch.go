package launch

import (
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

type Process struct {
	Type             string   `toml:"type" json:"type"`
	Command          string   `toml:"command" json:"command"`
	Args             []string `toml:"args" json:"args"`
	Direct           bool     `toml:"direct" json:"direct"`
	Default          bool     `toml:"default,omitempty" json:"default,omitempty"`
	BuildpackID      string   `toml:"buildpack-id" json:"buildpackID"`
	WorkingDirectory string   `toml:"working-dir,omitempty" json:"working-dir,omitempty"`
}

// I hope there's a better way to do this, but this gets the job done...
func (process *Process) populateCommonProcessFields(p interface{}) error {
	pp, ok := p.(map[string]interface{})
	if !ok {
		return fmt.Errorf("process has unexpected structure")
	}

	ptype, ok := pp["type"]
	if !ok {
		return fmt.Errorf("process does not have a type")
	}
	if process.Type, ok = ptype.(string); !ok {
		return fmt.Errorf("process.type is not a string")
	}

	args, ok := pp["args"].([]interface{})
	if !ok {
		process.Args = []string{}
	} else {
		process.Args = make([]string, len(args))
		for i, arg := range args {
			process.Args[i], ok = arg.(string)
			if !ok {
				return fmt.Errorf("arg is not a string: %v", arg)
			}
		}
	}

	direct, ok := pp["direct"]
	if !ok {
		process.Direct = false
	} else if process.Direct, ok = direct.(bool); !ok {
		return fmt.Errorf("process.direct is not a bool")
	}

	pdefault, ok := pp["default"]
	if !ok {
		process.Default = false
	} else {
		if process.Default, ok = pdefault.(bool); !ok {
			return fmt.Errorf("process.default is not a bool")
		}
	}

	buildpackid, ok := pp["buildpack-id"]
	if !ok {
		process.BuildpackID = ""
	} else if process.BuildpackID, ok = buildpackid.(string); !ok {
		return fmt.Errorf("process.buildpack-id is not a string")
	}

	workingdir, ok := pp["working-dir"]
	if !ok {
		process.WorkingDirectory = ""
	} else {
		if process.WorkingDirectory, ok = workingdir.(string); !ok {
			return fmt.Errorf("process.working-dir is not a string")
		}
	}

	return nil
}

func (process *Process) UnmarshalTOML(p interface{}) error {
	err := process.populateCommonProcessFields(p)
	if err != nil {
		return err
	}
	pp, ok := p.(map[string]interface{})
	if !ok {
		return fmt.Errorf("error parsing process")
	}
	switch command := (pp["command"]).(type) {
	case []interface{}:
		if len(command) < 1 {
			return fmt.Errorf("command has no elements")
		}
		process.Command, ok = command[0].(string)
		if !ok {
			return fmt.Errorf("command contains non-string %v", command[0])
		}
		commandArgs := make([]string, len(command)-1)
		for i, arg := range command[1:] {
			commandArgs[i], ok = arg.(string)
			if !ok {
				return fmt.Errorf("command contains non-string %v", arg)
			}
		}
		process.Args = append(commandArgs, process.Args...)
	case string:
		process.Command = command
	default:
		return fmt.Errorf("process command is unknown type %v", reflect.TypeOf(command))
	}
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
