package cmd

import (
	"fmt"
	"strings"

	"github.com/buildpacks/lifecycle/log"

	"github.com/buildpacks/lifecycle/api"
)

const (
	DeprecationModeQuiet = "quiet"
	DeprecationModeWarn  = "warn"
	DeprecationModeError = "error"
)

var DeprecationMode = EnvOrDefault(EnvDeprecationMode, DefaultDeprecationMode)

type APIVerifier struct{}

func (v *APIVerifier) VerifyBuildpackAPI(kind, name, requested string, logger log.Logger) error {
	return VerifyBuildpackAPI(kind, name, requested, logger)
}

func VerifyBuildpackAPI(kind, name, requested string, logger log.Logger) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return FailErrCode(
			nil,
			CodeIncompatibleBuildpackAPI,
			fmt.Sprintf("parse buildpack API '%s' for %s '%s'", requestedAPI, strings.ToLower(kind), name),
		)
	}
	if api.Buildpack.IsSupported(requestedAPI) {
		if api.Buildpack.IsDeprecated(requestedAPI) {
			switch DeprecationMode {
			case DeprecationModeQuiet:
				break
			case DeprecationModeError:
				logger.Errorf("%s '%s' requests deprecated API '%s'", kind, name, requested)
				logger.Errorf("Deprecated APIs are disabled by %s=%s", EnvDeprecationMode, DeprecationModeError)
				return buildpackAPIError(kind, name, requested)
			case DeprecationModeWarn:
				logger.Warnf("%s '%s' requests deprecated API '%s'", kind, name, requested)
			default:
				logger.Warnf("%s '%s' requests deprecated API '%s'", kind, name, requested)
			}
		}
		return nil
	}
	return buildpackAPIError(kind, name, requested)
}

func buildpackAPIError(moduleKind string, name string, requested string) error {
	return FailErrCode(
		fmt.Errorf("buildpack API version '%s' is incompatible with the lifecycle", requested),
		CodeIncompatibleBuildpackAPI,
		fmt.Sprintf("set API for %s '%s'", moduleKind, name),
	)
}

func VerifyPlatformAPI(requested string) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return FailErrCode(
			nil,
			CodeIncompatiblePlatformAPI,
			fmt.Sprintf("parse platform API '%s'", requested),
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

func platformAPIError(requested string) error {
	return FailErrCode(
		fmt.Errorf("platform API version '%s' is incompatible with the lifecycle", requested),
		CodeIncompatiblePlatformAPI,
		"set platform API",
	)
}
