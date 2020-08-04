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
	BuildpackID string   `toml:"buildpack-id" json:"buildpackID"`
}

func (p Process) Path() string {
	return filepath.Join(ProcessDir, p.Type+exe)
}

type Metadata struct {
	Processes  []Process   `toml:"processes" json:"processes"`
	Buildpacks []Buildpack `toml:"buildpacks" json:"buildpacks"`
}

func (m Metadata) FindProcessType(kind string) (Process, bool) {
	for _, p := range m.Processes {
		if p.Type == kind {
			return p, true
		}
	}
	return Process{}, false
}

type Buildpack struct {
	API string `toml:"api"`
	ID  string `toml:"id"`
}

type Env interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string) error
	List() []string
	Get(string) string
}

func EscapeID(id string) string {
	return strings.Replace(id, "/", "_", -1)
}

func GetMetadataFilePath(layersDir string) string {
	return path.Join(layersDir, "config", "metadata.toml")
}
