package launch

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
)

// ProcessFor creates a process from container cmd
//   If the Platform API if 0.4 or greater and DefaultProcess is set:
//     * The default process is returned with `cmd` appended to the process args
//   If the Platform API is less than 0.4
//     * If there is exactly one argument and it matches a process type, it returns that process.
//     * If cmd is empty, it returns the default process
//   Else
//     * it constructs a new process from cmd
//     * If the first element in cmd is `cmd` the process shall be direct
func (l *Launcher) ProcessFor(cmd []string) (Process, error) {
	if l.PlatformAPI.LessThan("0.4") {
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

	switch {
	case len(process.Command) > 1 && len(cmd) > 0: // process has always-provided args and there are user-provided args
		process.Args = process.Command[1:]          // always-provided args
		process.Args = append(process.Args, cmd...) // overridable args are omitted, user-provided args are appended
		process.Command = []string{process.Command[0]}
	case len(process.Command) > 1: // process has always-provided args but there are no user-provided args
		overridableArgs := process.Args
		process.Args = process.Command[1:]                      // always-provided args
		process.Args = append(process.Args, overridableArgs...) // overridable args are appended
		process.Command = []string{process.Command[0]}
	case len(cmd) == 0: // process does not have always-provided args and there are no user-provided args
		// nop, process args are provided
	default: // process does not have always-provided args and there are user-provided args
		// check buildpack API
		bp, err := l.buildpackForProcess(process)
		if err != nil {
			return Process{}, err
		}
		switch {
		case api.MustParse(bp.API).LessThan("0.9"):
			process.Args = append(process.Args, cmd...) // user-provided args are appended to process args
		default:
			process.Args = cmd // user-provided args replace process args
		}
	}

	return process, nil
}

func (l *Launcher) buildpackForProcess(process Process) (Buildpack, error) {
	for _, bp := range l.Buildpacks {
		if bp.ID == process.BuildpackID {
			return bp, nil
		}
	}
	return Buildpack{}, fmt.Errorf("failed to find buildpack for process %s with buildpack ID %s", process.Type, process.BuildpackID)
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

func (l *Launcher) findProcessType(pType string) (Process, bool) {
	for _, p := range l.Processes {
		if p.Type == pType {
			return p, true
		}
	}

	return Process{}, false
}

func (l *Launcher) userProvidedProcess(cmd []string) (Process, error) {
	if len(cmd) == 0 {
		return Process{}, errors.New("when there is no default process a command is required")
	}
	if len(cmd) > 1 && cmd[0] == "--" {
		return Process{Command: []string{cmd[1]}, Args: cmd[2:], Direct: true}, nil
	}

	return Process{Command: []string{cmd[0]}, Args: cmd[1:]}, nil
}

func getProcessWorkingDirectory(process Process, appDir string) string {
	if process.WorkingDirectory == "" {
		return appDir
	}
	return process.WorkingDirectory
}
