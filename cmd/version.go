package cmd

import (
	"fmt"

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
	if SCMCommit == "" {
		return Version
	}

	return fmt.Sprintf("%s+%s", Version, SCMCommit)
}

func VerifyPlatformAPI(requestedAPI string) error {
	if api.Platform.IsSupported(requestedAPI) {
		if api.Platform.IsDeprecated(requestedAPI) {
			switch DeprecationMode {
			case DeprecationModeQuiet:
				break
			case DeprecationModeError:
				return platformAPIError(requestedAPI)
			case DeprecationModeWarn:
				DefaultLogger.Warnf("Platform API '%s' is deprecated", requestedAPI)
			default:
				DefaultLogger.Warnf("Platform API '%s' is deprecated", requestedAPI)
			}
		}
		return nil
	}
	return platformAPIError(requestedAPI)
}

func platformAPIError(requestedAPI string) error {
	return FailErrCode(
		fmt.Errorf("set platform API: platform API version '%s' is incompatible with the lifecycle", requestedAPI),
		CodeIncompatible,
	)
}
