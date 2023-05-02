package env

import (
	"os"
	"strings"

	"github.com/buildpacks/lifecycle/platform/launch/env"
)

var LaunchEnvExcludelist = []string{
	env.VarAppDir,
	env.VarDeprecationMode,
	env.VarLayersDir,
	env.VarPlatformAPI,
	env.VarProcessType,
	// TODO: env.VarNoColor is missing from this list - is it intentional?
}

// NewLaunchEnv returns an Env for process launch from the given environ.
//
// Keys in the LaunchEnvExcludelist shall be removed.
// processDir will be removed from the beginning of PATH if present.
func NewLaunchEnv(environ []string, processDir string, lifecycleDir string) *Env {
	vars := varsFromEnv(environ, ignoreEnvVarCase, isExcluded)
	if path, ok := vars.vals["PATH"]; ok {
		parts := strings.Split(path, string(os.PathListSeparator))
		var stripped []string
		for _, part := range parts {
			if part == processDir || part == lifecycleDir {
				continue
			}
			stripped = append(stripped, part)
		}
		vars.vals["PATH"] = strings.Join(stripped, string(os.PathListSeparator))
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
