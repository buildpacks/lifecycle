package cmd

import (
	"fmt"
	"strings"

	"github.com/buildpacks/lifecycle/api"
)

// The following variables are injected at compile time.
var (
	// Version is the version of the lifecycle and all produced binaries.
	Version = "0.0.0"
	// SCMCommit is the commit information provided by SCM.
	SCMCommit = ""
	// SCMRepository is the source repository.
	SCMRepository = ""

	DeprecationMode = EnvOrDefault(EnvDeprecationMode, DefaultDeprecationMode)
)

const (
	DeprecationModeQuiet = "quiet"
	DeprecationModeWarn  = "warn"
	DeprecationModeError = "error"
)

// buildVersion is a display format of the version and build metadata in compliance with semver.
func buildVersion() string {
	// noinspection GoBoolExpressions
	if SCMCommit == "" || strings.Contains(Version, SCMCommit) {
		return Version
	}

	return fmt.Sprintf("%s+%s", Version, SCMCommit)
}

func VerifyPlatformAPI(requested string) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return FailErrCode(
			fmt.Errorf("parse platform API '%s'", requested),
			CodeIncompatiblePlatformAPI,
		)
	}
	if api.Platform.IsSupported(requestedAPI) {
		if api.Platform.IsDeprecated(requestedAPI) {
			switch DeprecationMode {
			case DeprecationModeQuiet:
				break
			case DeprecationModeError:
				DefaultLogger.Errorf("Platform requested deprecated API '%s'", requested)
				DefaultLogger.Errorf("Deprecated APIs are disabled by %s=%s", EnvDeprecationMode, DeprecationModeError)
				return platformAPIError(requested)
			case DeprecationModeWarn:
				DefaultLogger.Warnf("Platform requested deprecated API '%s'", requested)
			default:
				DefaultLogger.Warnf("Platform requested deprecated API '%s'", requested)
			}
		}
		return nil
	}
	return platformAPIError(requested)
}

type APIVerifier struct{}

func (v *APIVerifier) VerifyBuildpackAPIForBuildpack(name, requested string) error {
	return VerifyBuildpackAPI(Buildpack, name, requested)
}

func (v *APIVerifier) VerifyBuildpackAPIForExtension(name, requested string) error {
	return VerifyBuildpackAPI(Extension, name, requested)
}

type ModuleKind int

const (
	Buildpack ModuleKind = iota
	Extension
)

func VerifyBuildpackAPI(kind ModuleKind, name string, requested string) error {
	moduleKind := "Buildpack"
	if kind == Extension {
		moduleKind = "Extension"
	}
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return FailErrCode(
			fmt.Errorf("parse buildpack API '%s' for %s '%s'", requestedAPI, strings.ToLower(moduleKind), name),
			CodeIncompatibleBuildpackAPI,
		)
	}
	if api.Buildpack.IsSupported(requestedAPI) {
		if api.Buildpack.IsDeprecated(requestedAPI) {
			switch DeprecationMode {
			case DeprecationModeQuiet:
				break
			case DeprecationModeError:
				DefaultLogger.Errorf("%s '%s' requests deprecated API '%s'", moduleKind, name, requested)
				DefaultLogger.Errorf("Deprecated APIs are disabled by %s=%s", EnvDeprecationMode, DeprecationModeError)
				return buildpackAPIError(moduleKind, name, requested)
			case DeprecationModeWarn:
				DefaultLogger.Warnf("%s '%s' requests deprecated API '%s'", moduleKind, name, requested)
			default:
				DefaultLogger.Warnf("%s '%s' requests deprecated API '%s'", moduleKind, name, requested)
			}
		}
		return nil
	}
	return buildpackAPIError(moduleKind, name, requested)
}

func platformAPIError(requested string) error {
	return FailErrCode(
		fmt.Errorf("set platform API: platform API version '%s' is incompatible with the lifecycle", requested),
		CodeIncompatiblePlatformAPI,
	)
}

func buildpackAPIError(moduleKind string, name string, requested string) error {
	return FailErrCode(
		fmt.Errorf("set API for %s '%s': buildpack API version '%s' is incompatible with the lifecycle", moduleKind, name, requested),
		CodeIncompatibleBuildpackAPI,
	)
}
