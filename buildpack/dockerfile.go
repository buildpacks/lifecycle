package buildpack

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/log"
)

const (
	DockerfileKindBuild = "build"
	DockerfileKindRun   = "run"

	buildDockerfileName = "build.Dockerfile"
	runDockerfileName   = "run.Dockerfile"

	baseImageArgName = "base_image"
	baseImageArgRef  = "${base_image}"

	errArgumentsNotPermitted            = "run.Dockerfile should not expect arguments"
	errBuildMissingRequiredARGCommand   = "build.Dockerfile did not start with required ARG command"
	errBuildMissingRequiredFROMCommand  = "build.Dockerfile did not contain required FROM ${base_image} command"
	errMissingRequiredStage             = "%s should have at least one stage"
	errMultiStageNotPermitted           = "%s is not permitted to use multistage build"
	errRunOtherInstructionsNotPermitted = "run.Dockerfile is not permitted to have instructions other than FROM"
	warnCommandNotRecommended           = "%s command %s on line %d is not recommended"
)

var permittedCommands = []string{"FROM", "ADD", "ARG", "COPY", "ENV", "LABEL", "RUN", "SHELL", "USER", "WORKDIR"}

type DockerfileInfo struct {
	ExtensionID string
	Kind        string
	Path        string
	NewBase     string
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
	d, err = os.ReadFile(dockerfile)
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

func VerifyBuildDockerfile(dockerfile string, logger log.Logger) error {
	stages, margs, err := parseDockerfile(dockerfile)
	if err != nil {
		return err
	}

	// validate only 1 FROM
	if len(stages) > 1 {
		return fmt.Errorf(errMultiStageNotPermitted, buildDockerfileName)
	}

	// validate only permitted Commands
	for _, stage := range stages {
		for _, command := range stage.Commands {
			found := false
			for _, permittedCommand := range permittedCommands {
				if permittedCommand == strings.ToUpper(command.Name()) {
					found = true
					break
				}
			}
			if !found {
				logger.Warnf(warnCommandNotRecommended, buildDockerfileName, strings.ToUpper(command.Name()), command.Location()[0].Start.Line)
			}
		}
	}

	// validate build.Dockerfile preamble
	if len(margs) != 1 {
		return errors.New(errBuildMissingRequiredARGCommand)
	}
	if margs[0].Args[0].Key != baseImageArgName {
		return errors.New(errBuildMissingRequiredARGCommand)
	}
	// sanity check to prevent panic
	if len(stages) == 0 {
		return fmt.Errorf(errMissingRequiredStage, buildDockerfileName)
	}

	if stages[0].BaseName != baseImageArgRef {
		return errors.New(errBuildMissingRequiredFROMCommand)
	}

	return nil
}

func VerifyRunDockerfile(dockerfile string, buildpackAPI *api.Version, logger log.Logger) (string, error) {
	if buildpackAPI.LessThan("0.10") {
		return verifyRunDockerfile09(dockerfile)
	}
	return verifyRunDockerfile(dockerfile, logger)
}

func verifyRunDockerfile(dockerfile string, logger log.Logger) (string, error) {
	stages, _, err := parseDockerfile(dockerfile)
	if err != nil {
		return "", err
	}

	// validate only 1 FROM
	if len(stages) > 1 {
		return "", fmt.Errorf(errMultiStageNotPermitted, runDockerfileName)
	}
	if len(stages) == 0 {
		return "", fmt.Errorf(errMissingRequiredStage, runDockerfileName)
	}

	var newBase string
	// validate only permitted Commands
	for _, stage := range stages {
		if stage.BaseName != baseImageArgRef {
			newBase = stage.BaseName
		}
		for _, command := range stage.Commands {
			found := false
			for _, permittedCommand := range permittedCommands {
				if permittedCommand == strings.ToUpper(command.Name()) {
					found = true
					break
				}
			}
			if !found {
				logger.Warnf(warnCommandNotRecommended, runDockerfileName, strings.ToUpper(command.Name()), command.Location()[0].Start.Line)
			}
		}
	}

	return newBase, nil
}

func verifyRunDockerfile09(dockerfile string) (string, error) {
	stages, margs, err := parseDockerfile(dockerfile)
	if err != nil {
		return "", err
	}

	// validate only 1 FROM
	if len(stages) > 1 {
		return "", fmt.Errorf(errMultiStageNotPermitted, runDockerfileName)
	}

	// validate FROM does not expect argument
	if len(margs) > 0 {
		return "", errors.New(errArgumentsNotPermitted)
	}

	// sanity check to prevent panic
	if len(stages) == 0 {
		return "", fmt.Errorf(errMissingRequiredStage, runDockerfileName)
	}

	// validate no instructions in stage
	if len(stages[0].Commands) != 0 {
		return "", fmt.Errorf(errRunOtherInstructionsNotPermitted)
	}

	return stages[0].BaseName, nil
}
