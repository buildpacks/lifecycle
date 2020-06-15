package env

var LaunchEnvBlocklist = []string{
	"CNB_LAYERS_DIR",
	"CNB_APP_DIR",
	"CNB_PROCESS_TYPE",
}

func NewLaunchEnv(environ []string) *Env {
	return &Env{
		RootDirMap: POSIXLaunchEnv,
		Vars:       varsFromEnviron(environ, isBlocklisted),
	}
}

func isBlocklisted(k string) bool {
	for _, wk := range LaunchEnvBlocklist {
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
