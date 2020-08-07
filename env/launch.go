package env

import (
	"os"
	"strings"
)

var LaunchEnvExcludelist = []string{
	"CNB_LAYERS_DIR",
	"CNB_APP_DIR",
	"CNB_PROCESS_TYPE",
	"CNB_PLATFORM_API",
	"CNB_DEPRECATION_MODE",
}

// NewLaunchEnv returns an Env for process launch from the given environ.
//
// Keys in the LaunchEnvExcludelist shall be removed.
// processDir will be removed from the beginning of PATH if present.
func NewLaunchEnv(environ []string, processDir string) *Env {
	vars := varsFromEnviron(environ, ignoreEnvVarCase, isExcluded)
	if path, ok := vars.vals["PATH"]; ok {
		pathElems := strings.Split(path, string(os.PathListSeparator))
		if pathElems[0] == processDir {
			vars.Set("PATH", strings.Join(pathElems[1:], string(os.PathListSeparator)))
		}
	}
	return &Env{
		RootDirMap: POSIXLaunchEnv,
		Vars:       vars,
	}
}

func isExcluded(k string) bool {
	for _, wk := range LaunchEnvExcludelist {
		if matches(wk, k) {
			return true
		}
	}
	return false
}

var POSIXLaunchEnv = map[string][]string{
	"bin": {"PATH"},
	"lib": {"LD_LIBRARY_PATH"},
}
