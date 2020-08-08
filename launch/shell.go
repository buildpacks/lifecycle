package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
)

type Shell interface {
	Launch(ShellProcess) error
}

type ShellProcess struct {
	Script   bool // Script indicates whether Command is a script or should be a token in a generated script
	Args     []string
	Command  string
	Caller   string // Caller used to set argv0 for Bash profile scripts and is ignored in Cmd
	Profiles []string
	Env      []string
}

func (l *Launcher) launchWithShell(self string, process Process) error {
	profs, err := l.profiles(process)
	if err != nil {
		return errors.Wrap(err, "find profiles")
	}
	script, err := l.isScript(process)
	if err != nil {
		return err
	}
	return l.Shell.Launch(ShellProcess{
		Script:   script,
		Caller:   self,
		Command:  process.Command,
		Args:     process.Args,
		Profiles: profs,
		Env:      l.Env.List(),
	})
}

func (l *Launcher) profiles(process Process) ([]string, error) {
	var profiles []string

	appendIfFile := func(path string) error {
		fi, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			profiles = append(profiles, path)
		}
		return nil
	}
	layersDir, err := filepath.Abs(l.LayersDir)
	if err != nil {
		return nil, err
	}
	for _, bp := range l.Buildpacks {
		globPaths := []string{filepath.Join(layersDir, EscapeID(bp.ID), "*", "profile.d", profileGlob)}
		if process.Type != "" {
			globPaths = append(globPaths, filepath.Join(layersDir, EscapeID(bp.ID), "*", "profile.d", process.Type, profileGlob))
		}
		var scripts []string
		for _, globPath := range globPaths {
			matches, err := filepath.Glob(globPath)
			if err != nil {
				return nil, err
			}
			scripts = append(scripts, matches...)
		}
		for _, script := range scripts {
			if err := appendIfFile(script); err != nil {
				return nil, err
			}
		}
	}

	if err := appendIfFile(filepath.Join(l.AppDir, appProfile)); err != nil {
		return nil, err
	}

	return profiles, nil
}

func (l *Launcher) isScript(process Process) (bool, error) {
	if runtime.GOOS == "windows" {
		// Windows does not support script commands
		return false, nil
	}
	if len(process.Args) == 0 {
		return true, nil
	}
	if process.BuildpackID == "" {
		return false, nil
	}
	for _, bp := range l.Buildpacks {
		if bp.ID != process.BuildpackID {
			continue
		}
		bpAPI, err := api.NewVersion(bp.API)
		if err != nil {
			return false, fmt.Errorf("failed to parse api '%s' of buildpack '%s'", bp.API, bp.ID)
		}
		if isLegacyProcess(bpAPI) {
			return true, nil
		}
		return false, nil
	}
	return false, fmt.Errorf("process type '%s' provided by unknown buildpack '%s'", process.Type, process.BuildpackID)
}

func isLegacyProcess(bpAPI *api.Version) bool {
	return bpAPI.Compare(api.MustParse("0.4")) == -1
}
