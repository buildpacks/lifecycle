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

	// SupportedPlatformAPIs contains a comma separated list of supported platforms APIs
	SupportedPlatformAPIs string
	// DepreactedPlatformAPIs contains a comma separated list of depreacted platforms APIs
	DeprecatedPlatformAPIs string

	EnvPlatformAPI     = "CNB_PLATFORM_API"
	EnvDeprecationMode = "CNB_DEPRECATION_MODE"

	// DefaultPlatformAPI specifies platform API to provide when "CNB_PLATFORM_API" is unset
	DefaultPlatformAPI     = "0.3"
	DefaultDeprecationMode = "warn"
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

func VerifyCompatibility() error {
	apis, _ := api.NewAPIs(splitApis(SupportedPlatformAPIs), splitApis(DeprecatedPlatformAPIs))

	requestedAPI := envOrDefault(EnvPlatformAPI, DefaultPlatformAPI)
	if apis.IsSupported(requestedAPI) {
		if apis.IsDeprecated(requestedAPI) {
			switch envOrDefault(EnvDeprecationMode, DeprecationModeWarn) {
			case DeprecationModeQuiet:
				break
			case DeprecationModeError:
				return platformAPIError(requestedAPI)
			case DeprecationModeWarn:
				Logger.Warnf("Platform API '%s' is deprecated", requestedAPI)
			default:
				Logger.Warnf("Platform API '%s' is deprecated", requestedAPI)
			}
		}
		return nil
	}
	return platformAPIError(requestedAPI)
}

func splitApis(joined string) []string {
	supported := strings.Split(joined, `,`)
	if len(supported) == 1 && supported[0] == "" {
		supported = nil
	}
	return supported
}

func platformAPIError(requestedAPI string) error {
	return FailErrCode(
		fmt.Errorf("set platform API: platform API version '%s' is incompatible with the lifecycle", requestedAPI),
		CodeIncompatible,
	)
}
