package launch

import (
	"path"
	"path/filepath"
	"strings"
)

type Process struct {
	Type        string   `toml:"type" json:"type"`
	Command     string   `toml:"command" json:"command"`
	Args        []string `toml:"args" json:"args"`
	Direct      bool     `toml:"direct" json:"direct"`
	Default     bool     `toml:"default, omitzero" json:"default"`
	BuildpackID string   `toml:"buildpack-id" json:"buildpackID"`
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

func (m Metadata) FindLastDefaultProcessType() (Process, bool) {
	defaultFound := false
	var lastDefaultProcess Process
	for _, p := range m.Processes {
		if p.Default {
			lastDefaultProcess = p
			defaultFound = true
		}
	}
	return lastDefaultProcess, defaultFound
}

type Buildpack struct {
	API string `toml:"api"`
	ID  string `toml:"id"`
}

func EscapeID(id string) string {
	return strings.Replace(id, "/", "_", -1)
}

func GetMetadataFilePath(layersDir string) string {
	return path.Join(layersDir, "config", "metadata.toml")
}
