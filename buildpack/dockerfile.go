package buildpack

import (
	"path/filepath"
)

type Dockerfile struct {
	// ExtensionID   string // TODO: see if this is needed
	Path  string
	Build bool
	Run   bool
}

func processDockerfiles(bpOutputDir string) ([]Dockerfile, error) {
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
				Path: m,
				Run:  true,
			})
			continue
		}

		if filename == "build.Dockerfile" {
			dockerfiles = append(dockerfiles, Dockerfile{
				Path:  m,
				Build: true,
			})
			continue
		}

		if filename == "Dockerfile" {
			dockerfiles = append(dockerfiles, Dockerfile{
				Path:  m,
				Build: true,
				Run:   true,
			})
			continue
		}
		// ignore other glob matches e.g., some-random.Dockerfile
	}

	return dockerfiles, nil
}
