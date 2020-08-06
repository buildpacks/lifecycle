package launch

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
)

// ProcessFor creates a process from cmd
func (l *Launcher) ProcessFor(cmd []string) (Process, error) {
	if l.PlatformAPI.Compare(api.MustParse("0.4")) < 0 {
		return l.processForLegacy(cmd)
	}

	if l.DefaultProcessType == "" {
		process, err := l.userProvidedProcess(cmd)
		if err != nil {
			return Process{}, err
		}
		return process, nil
	}

	process, ok := l.findProcessType(l.DefaultProcessType)
	if !ok {
		return Process{}, fmt.Errorf("process type %s was not found", l.DefaultProcessType)
	}
	process.Args = append(process.Args, cmd...)

	return process, nil
}

func (l *Launcher) processForLegacy(cmd []string) (Process, error) {
	if len(cmd) == 0 {
		if process, ok := l.findProcessType(l.DefaultProcessType); ok {
			return process, nil
		}

		return Process{}, fmt.Errorf("process type %s was not found", l.DefaultProcessType)
	}

	if len(cmd) == 1 {
		if process, ok := l.findProcessType(cmd[0]); ok {
			return process, nil
		}
	}

	return l.userProvidedProcess(cmd)
}

func (l *Launcher) userProvidedProcess(cmd []string) (Process, error) {
	if len(cmd) == 0 {
		return Process{}, errors.New("when there is no default process a command is required")
	}
	if len(cmd) > 1 && cmd[0] == "--" {
		return Process{Command: cmd[1], Args: cmd[2:], Direct: true}, nil
	}

	return Process{Command: cmd[0], Args: cmd[1:]}, nil
}
