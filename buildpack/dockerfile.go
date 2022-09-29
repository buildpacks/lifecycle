package buildpack

import (
	"bufio"
	"fmt"
	"os"
	"strings"
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

type InstructionCallback func(verb string, arg string, lineno int, state map[string]string) (string, error)

func walkDockerInstructions(filename string, handleInstruction InstructionCallback) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var inContinueBlock bool = false
	var checkLine bool = true
	var lineNo = 0
	var state = make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		lineNo++

		//skip comments (docker permits whitespace before #)
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix("#", trimmedLine) {
			continue
		}

		//does this line start a continuation block?
		if !inContinueBlock {
			checkLine = true
		} else {
			checkLine = false
		}
		if strings.HasSuffix("\\", line) {
			inContinueBlock = true
		} else {
			inContinueBlock = false
		}

		//if were not in a continuance, then verify the verb at the start of the line
		if checkLine {
			split := strings.Split(trimmedLine, " ")
			verbFromLine := split[0]
			firstArg := split[1]
			result, err := handleInstruction(verbFromLine, firstArg, lineNo, state)
			if err != nil {
				return "", err
			} else {
				if result != "" {
					return result, nil
				}
			}
		}
	}

	//if the scanner broke (file too large?) report that & fail.
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

func verifyRunVerbs(verb string, arg string, lineno int, state map[string]string) (string, error) {
	var allowed = []string{"FROM"}
	var found bool = false
	for _, validVerb := range allowed {
		if validVerb == verb {
			found = true
			if verb == "FROM" {
				if state["FROM"] == "found" {
					return "", fmt.Errorf("failed to validate Dockerfile, only one FROM instruction is permitted")
				} else {
					state["FROM"] = "found"
				}
			}
			break
		}
	}
	if !found {
		return "", fmt.Errorf("failed to validate Dockerfile, instruction '%s' on line '%d' is not permitted", verb, lineno)
	} else {
		return "", nil
	}
}

func verifyBuildVerbs(verb string, arg string, lineno int, state map[string]string) (string, error) {
	var allowed = []string{"FROM", "ADD", "ARG", "COPY", "ENV", "LABEL", "RUN", "SHELL", "USER", "WORKDIR"}
	var found bool = false
	for _, validVerb := range allowed {
		if validVerb == verb {
			found = true
			if verb == "FROM" {
				if state["FROM"] == "found" {
					return "", fmt.Errorf("failed to validate Dockerfile, only one FROM instruction is permitted")
				} else {
					state["FROM"] = "found"
				}
			}
			break
		}
	}
	if !found {
		return "", fmt.Errorf("failed to validate Dockerfile, instruction '%s' on line '%d' is not permitted", verb, lineno)
	} else {
		return "", nil
	}
}

func retrieveFirstFromArg(verb string, arg string, lineno int, state map[string]string) (string, error) {
	if verb == "FROM" {
		return arg, nil
	} else {
		return "", nil
	}
}

func verifyBuildDockerFilePreamble(verb string, arg string, lineno int, state map[string]string) (string, error) {
	//expect first instruction to be 'ARG'
	if state["foundArg"] == "" {
		if verb != "ARG" {
			return "", fmt.Errorf("build Dockerfile MUST start with ARG instruction, instead found '%s' on line '%d'", verb, lineno)
		} else {
			state["foundArg"] = "true"
			if arg != "base_image" {
				return "", fmt.Errorf("build Dockerfile MUST start with ARG base_image, instead found '%s %s' on line '%d", verb, arg, lineno)
			}
		}
	} else {
		//expect second instruction to be 'FROM'
		if verb != "FROM" {
			return "", fmt.Errorf("build Dockerfile MUST start with FROM instruction follwing ARG, instead found '%s %s'", verb, arg)
		} else {
			if arg != "${base_image}" {
				return "", fmt.Errorf("build Dockerfile MUST have FROM ${base_image}, instead found '%s %s'", verb, arg)
			} else {
				return "DONE", nil
			}
		}
	}
	return "", nil
}

func VerifyBuildDockerfile(dockerfile string) error {
	_, err := walkDockerInstructions(dockerfile, verifyBuildVerbs)
	if err != nil {
		return err
	}
	arg, err2 := walkDockerInstructions(dockerfile, verifyBuildDockerFilePreamble)
	if err2 != nil {
		return err2
	}
	if arg != "DONE" {
		return fmt.Errorf("failed to locate FROM instruction in Dockerfile")
	}
	return nil
}

func VerifyRunDockerfile(dockerfile string) error {
	_, err := walkDockerInstructions(dockerfile, verifyRunVerbs)
	return err
}

func RetrieveFirstFromImageNameFromDockerfile(dockerfile string) (string, error) {
	arg, err := walkDockerInstructions(dockerfile, retrieveFirstFromArg)
	if arg == "" {
		return "", fmt.Errorf("failed to locate FROM instruction in Dockerfile")
	} else {
		return arg, err
	}
}
