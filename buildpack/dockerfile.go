package buildpack

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

const (
	DockerfileKindBuild = "build"
	DockerfileKindRun   = "run"
)

type DockerfileInfo struct {
	ExtensionID string
	Kind        string
	Path        string
}

type ExtendConfig struct {
	Build ExtendBuildConfig `toml:"build"`
}

type ExtendBuildConfig struct {
	Args []ExtendArg `toml:"args"`
}

type ExtendArg struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

func parseDockerfile(dockerfile string) ([]instructions.Stage, []instructions.ArgCommand, error) {
	var err error
	var d []uint8
	d, err = ioutil.ReadFile(dockerfile)
	if err != nil {
		return nil, nil, err
	}
	p, err := parser.Parse(bytes.NewReader(d))
	if err != nil {
		return nil, nil, err
	}
	stages, metaArgs, err := instructions.Parse(p.AST)
	if err != nil {
		return nil, nil, err
	}
	return stages, metaArgs, nil
}

func VerifyBuildDockerfile(dockerfile string) error {
	stages, margs, err := parseDockerfile(dockerfile)
	if err != nil {
		return err
	}

	//validate only 1 FROM
	if len(stages) > 1 {
		return fmt.Errorf("build.Dockerfile is not permitted to use multistage build")
	}

	//validate only permitted Commands
	var permitted = []string{"from", "add", "arg", "copy", "env", "label", "run", "shell", "user", "workdir"}
	for _, stage := range stages {
		for _, command := range stage.Commands {
			var found bool = false
			for _, permittedCommand := range permitted {
				if permittedCommand == command.Name() {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("build.Dockerfile command %s on line %d is not permitted", command.Name(), command.Location()[0].Start.Line)
			}
		}
	}

	//validate build.Dockerfile preamble
	if len(margs) != 1 {
		return fmt.Errorf("build.Dockerfile did not start with required ARG command")
	}
	if margs[0].Args[0].Key != "base_image" {
		return fmt.Errorf("build.Dockerfile did not start with required ARG base_image command")
	}
	if stages[0].BaseName != "${base_image}" {
		return fmt.Errorf("build.Dockerfile did not contain required FROM ${base_image} command")
	}

	return nil
}

func VerifyRunDockerfile(dockerfile string) error {
	stages, _, err := parseDockerfile(dockerfile)
	if err != nil {
		return err
	}

	//validate only 1 FROM
	if len(stages) > 1 {
		return fmt.Errorf("run.Dockerfile is not permitted to use multistage build")
	}

	//validate no instructions in stage
	if len(stages[0].Commands) != 0 {
		return fmt.Errorf("run.Dockerfile is not permitted to have instructions other than FROM")
	}

	return nil
}

func RetrieveFirstFromImageNameFromDockerfile(dockerfile string) (string, error) {
	ins, _, err := parseDockerfile(dockerfile)
	if err != nil {
		return "", err
	}
	return ins[0].BaseName, nil
}
