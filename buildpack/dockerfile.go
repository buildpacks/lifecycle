package buildpack

import (
	"path/filepath"
)

type Dockerfile struct {
	ExtensionID string          `toml:"extension_id"` // TODO: nest [[dockerfiles]] under [[extensions]]?
	Path        string          `toml:"path"`
	Build       bool            `toml:"build"`
	Run         bool            `toml:"run"`
	Args        []DockerfileArg `toml:"args"`
}

type DockerfileArg struct {
	Key   string `toml:"name"` // TODO: which do we want?
	Value string `toml:"value"`
}

func processDockerfiles(bpOutputDir, extID string, buildArgs, runArgs []DockerfileArg) ([]Dockerfile, error) {
	var (
		dockerfileGlob = filepath.Join(bpOutputDir, "*Dockerfile")
		dockerfiles    []Dockerfile
	)

	matches, err := filepath.Glob(dockerfileGlob)
	if err != nil {
		return nil, err
	}

	for _, m := range matches {
		_, filename := filepath.Split(m)

		if filename == "run.Dockerfile" {
			dockerfiles = append(dockerfiles, Dockerfile{
				ExtensionID: extID,
				Path:        m,
				Run:         true,
				Args:        runArgs,
			})
			continue
		}

		if filename == "build.Dockerfile" {
			dockerfiles = append(dockerfiles, Dockerfile{
				ExtensionID: extID,
				Path:        m,
				Build:       true,
				Args:        buildArgs,
			})
			continue
		}

		if filename == "Dockerfile" {
			dockerfiles = append(dockerfiles, Dockerfile{
				ExtensionID: extID,
				Path:        m,
				Build:       true,
				Run:         true,
				Args:        append(buildArgs, runArgs...),
			})
			continue
		}
		// ignore other glob matches e.g., some-random.Dockerfile
	}

	return dockerfiles, nil
}
