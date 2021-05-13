package env

import (
	"runtime"
	"strings"
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

//go:generate mockgen -package testmock -destination testmock/platform.go github.com/buildpacks/lifecycle/env Platform

// Platform represents capabilities supported by the platform, it may be used to alter the env
// to fit supported features.
type Platform interface {
	SupportsAssetPackages() bool
}

// AssetsEnvVars is a list of environment variables to be included in order to support asset packages.
var AssetsEnvVars = []string{
	"CNB_ASSETS", // will be included in the build env when platform API >= 0.7
}

var ignoreEnvVarCase = runtime.GOOS == "windows"

// NewBuildEnv returns an build-time Env from the given environment.
//
// Keys in the BuildEnvIncludelist will be added to the Environment.
// if the platform supports asset packages, keys from AssetsEnvVars will be added to the environment.
func NewBuildEnv(environ []string, platform Platform) *Env {
	envFilter := inMatchList(BuildEnvIncludelist, flattenMap(POSIXBuildEnv))
	if platform.SupportsAssetPackages() {
		envFilter = inMatchList(BuildEnvIncludelist, flattenMap(POSIXBuildEnv), AssetsEnvVars)
	}
	return &Env{
		RootDirMap: POSIXBuildEnv,
		Vars:       varsFromEnv(environ, ignoreEnvVarCase, envFilter),
	}
}

// NewDetectEnv returns an detect-time Env from the given environment.
//
// only keys in the BuildEnvIncludelist will be added to the Environment.
func NewDetectEnv(environ []string) *Env {
	envFilter := inMatchList(append(BuildEnvIncludelist, flattenMap(POSIXBuildEnv)...))

	return &Env{
		RootDirMap: POSIXBuildEnv,
		Vars:       varsFromEnv(environ, ignoreEnvVarCase, envFilter),
	}
}

func matches(k1, k2 string) bool {
	if ignoreEnvVarCase {
		k1 = strings.ToUpper(k1)
		k2 = strings.ToUpper(k2)
	}
	return k1 == k2
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

// removeFilter is generic function used to filter env vars out of the environment
// if removeFilter(key) returns true, then the env var will be removed from the respective environment
type removeFilter func(key string) bool

func inMatchList(lists ...[]string) removeFilter {
	return func(key string) bool {
		for _, list := range lists {
			for _, wk := range list {
				if matches(wk, key) {
					// keep in env
					return false
				}
			}
		}
		return true
	}
}

func flattenMap(m map[string][]string) []string {
	result := make([]string, 0)
	for _, subList := range m {
		result = append(result, subList...)
	}

	return result
}
