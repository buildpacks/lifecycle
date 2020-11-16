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
	profiles, err := l.buildpackFiles(process, ProfileDDirName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find all profile scripts in layers dir, '%s'", l.LayersDir)
	}

	fi, err := os.Stat(filepath.Join(l.AppDir, appProfile))
	if os.IsNotExist(err) {
		return profiles, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to determine if app profile script exists at path '%s'", filepath.Join(l.AppDir, appProfile))
	}
	if !fi.IsDir() {
		profiles = append(profiles, appProfile)
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
