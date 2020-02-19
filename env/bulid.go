package env

import "strings"

var BuildEnvWhitelist = []string{"CNB_STACK_ID"}

func NewBuildEnv(environ []string) *Env {
	vars := make(map[string]string)
	for _, kv := range environ {
		parts := strings.Split(kv, "=")
		if len(parts) != 2 {
			continue
		}
		if !isWhitelisted(parts[0]) {
			continue
		}
		vars[parts[0]] = parts[1]
	}
	return &Env{
		RootDirMap: POSIXBuildEnv,
		vars:       vars,
	}
}

func isWhitelisted(k string) bool {
	for _, wk := range BuildEnvWhitelist {
		if wk == k {
			return true
		}
	}
	return false
}

var POSIXBuildEnv = map[string][]string{
	"bin": {
		"PATH",
	},
	"lib": {
		"LD_LIBRARY_PATH",
		"LIBRARY_PATH",
	},
	"include": {
		"CPATH",
	},
	"pkgconfig": {
		"PKG_CONFIG_PATH",
	},
}
