package buildpack

import (
	"path/filepath"
)

type Dockerfile struct {
	ExtensionID string
	Path        string
	Build       bool
	Run         bool
	Args        []DockerfileArg
}

type DockerfileArg struct {
	Key   string `toml:"key"`
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
