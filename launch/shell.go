package launch

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

func (l *Launcher) launchWithShell(self string, process Process) error {
	profiles, err := l.profiles(process)
	if err != nil {
		return errors.Wrap(err, "find profiles")
	}
	return l.execWithShell(self, process, profiles)
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
