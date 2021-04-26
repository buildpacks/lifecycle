package env

import (
	"runtime"
	"strings"

	"github.com/buildpacks/lifecycle/api"
)

var BuildEnvIncludelist = []string{
	"CNB_STACK_ID",
	"HOSTNAME",
	"HOME",
	"HTTPS_PROXY",
	"https_proxy",
	"HTTP_PROXY",
	"http_proxy",
	"NO_PROXY",
	"no_proxy",
}

// List of environment variables to be included for
// particular platform API versions
var BuildEnvIncludeAssets = []string{
	"CNB_ASSETS", // will be included in the build env when platform API > 0.8
}

var ignoreEnvVarCase = runtime.GOOS == "windows"

// NewBuildEnv returns an build-time Env from the given environment.
//
// Keys in the BuildEnvIncludelist will be added to the Environment.
// if platformAPI >= 0.8 keys from BuildEnvIncludeAssets will be added to the environment
func NewBuildEnv(environ []string, platformAPI *api.Version) *Env {
	return &Env{
		RootDirMap: POSIXBuildEnv,
		Vars:       varsFromEnv(environ, ignoreEnvVarCase, isNotIncludedForAPI(platformAPI)),
	}
}

// NewBuildEnv returns an detect-time Env from the given environment.
//
// only keys in the BuildEnvIncludelist will be added to the Environment.
func NewDetectEnv(environ []string) *Env {
	return &Env{
		RootDirMap: POSIXBuildEnv,
		Vars:       varsFromEnv(environ, ignoreEnvVarCase, isNotIncluded),
	}
}

func matches(k1, k2 string) bool {
	if ignoreEnvVarCase {
		k1 = strings.ToUpper(k1)
		k2 = strings.ToUpper(k2)
	}
	return k1 == k2
}

func isNotIncludedForAPI(platformAPI *api.Version) func(k string) bool {
	if platformAPI.Compare(api.MustParse("0.8")) < 0 {
		return isNotIncluded
	}

	return func(k string) bool {
		return isNotIncluded(k) && isNotIncludedForAssets(k)
	}
}

func isNotIncludedForAssets(k string) bool {
	for _, wk := range BuildEnvIncludeAssets {
		if matches(wk, k) {
			return false
		}
	}

	return true
}

func isNotIncluded(k string) bool {
	for _, wk := range BuildEnvIncludelist {
		if matches(wk, k) {
			return false
		}
	}
	for _, wks := range POSIXBuildEnv {
		for _, wk := range wks {
			if matches(wk, k) {
				return false
			}
		}
	}
	return true
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
