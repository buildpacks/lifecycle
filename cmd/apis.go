package cmd

import (
	"fmt"
	"strings"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
)

var DeprecationMode = EnvOrDefault(platform.EnvDeprecationMode, platform.DefaultDeprecationMode)

type BuildpackAPIVerifier struct{}

func (v *BuildpackAPIVerifier) VerifyBuildpackAPI(kind, name, requested string, logger log.Logger) error {
	return VerifyBuildpackAPI(kind, name, requested, logger)
}

func VerifyBuildpackAPI(kind, name, requested string, logger log.Logger) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return FailErrCode(
			fmt.Errorf("parse buildpack API '%s' for %s '%s'", requestedAPI, strings.ToLower(kind), name),
			platform.CodeForIncompatibleBuildpackAPI,
		)
	}
	if api.Buildpack.IsSupported(requestedAPI) {
		if api.Buildpack.IsDeprecated(requestedAPI) {
			switch DeprecationMode {
			case platform.ModeQuiet:
				break
			case platform.ModeError:
				logger.Errorf("%s '%s' requests deprecated API '%s'", kind, name, requested)
				logger.Errorf("Deprecated APIs are disabled by %s=%s", platform.EnvDeprecationMode, platform.ModeError)
				return buildpackAPIError(kind, name, requested)
			case platform.ModeWarn:
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
		fmt.Errorf("set API for %s '%s': buildpack API version '%s' is incompatible with the lifecycle", moduleKind, name, requested),
		platform.CodeForIncompatibleBuildpackAPI,
	)
}

func VerifyPlatformAPI(requested string, logger log.Logger) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return platformAPIError(requested)
	}
	if api.Platform.IsSupported(requestedAPI) {
		if api.Platform.IsDeprecated(requestedAPI) {
			switch DeprecationMode {
			case platform.ModeQuiet:
				break
			case platform.ModeError:
				logger.Errorf("Platform requested deprecated API '%s'", requested)
				logger.Errorf("Deprecated APIs are disabled by %s=%s", platform.EnvDeprecationMode, platform.ModeError)
				return platformAPIError(requested)
			case platform.ModeWarn:
				logger.Warnf("Platform requested deprecated API '%s'", requested)
			default:
				logger.Warnf("Platform requested deprecated API '%s'", requested)
			}
		}
		return nil
	}
	return platformAPIError(requested)
}

func platformAPIError(requested string) error {
	return FailErrCode(
		fmt.Errorf("set platform API: platform API version '%s' is incompatible with the lifecycle", requested),
		platform.CodeForIncompatiblePlatformAPI,
	)
}
