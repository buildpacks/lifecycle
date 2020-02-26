package env

var LaunchEnvBlacklist = []string{
	"CNB_LAYERS_DIR",
	"CNB_APP_DIR",
	"CNB_PROCESS_TYPE",
}

func NewLaunchEnv(environ []string) *Env {
	return &Env{
		RootDirMap: POSIXLaunchEnv,
		Vars:       varsFromEnviron(environ, isBlacklisted),
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
