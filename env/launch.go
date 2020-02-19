package env

import "strings"

var LaunchEnvBlacklist = []string{
	"CNB_LAYERS_DIR",
	"CNB_APP_DIR",
	"CNB_PROCESS_TYPE",
}

func NewLaunchEnv(environ []string) *Env {
	vars := make(map[string]string)
	for _, kv := range environ {
		parts := strings.Split(kv, "=")
		if len(parts) != 2 {
			continue
		}
		if isBlacklisted(parts[0]) {
			continue
		}
		vars[parts[0]] = parts[1]
	}
	return &Env{
		RootDirMap: POSIXLaunchEnv,
		vars:       vars,
	}
}

func isBlacklisted(k string) bool {
	for _, wk := range LaunchEnvBlacklist {
		if wk == k {
			return true
		}
	}
	return false
}

var POSIXLaunchEnv = map[string][]string{
	"bin": {"PATH"},
	"lib": {"LD_LIBRARY_PATH"},
}
