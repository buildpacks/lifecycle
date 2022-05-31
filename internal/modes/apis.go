package modes

import (
	"fmt"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/env"
	"github.com/buildpacks/lifecycle/internal/log"
)

const (
	EnvDeprecation      = "CNB_DEPRECATION_MODE"
	EnvExperimentalAPIs = "CNB_EXPERIMENTAL_MODE"

	DefaultDeprecation      = Warn
	DefaultExperimentalAPIs = Warn
)

var (
	Deprecation      = env.OrDefault(EnvDeprecation, DefaultDeprecation)
	ExperimentalAPIs = env.OrDefault(EnvExperimentalAPIs, DefaultExperimentalAPIs)
)

func VerifyPlatformAPI(requested string, apis api.APIs, logger log.Logger) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return fmt.Errorf("failed to parse platform API '%s'", requested)
	}
	if apis.IsSupported(requestedAPI) {
		if apis.IsDeprecated(requestedAPI) {
			switch Deprecation {
			case Quiet:
				break
			case Error:
				logger.Errorf("Platform requested deprecated API '%s'", requested)
				logger.Errorf("Deprecated APIs are disabled by %s=%s", EnvDeprecation, Error)
				return platformAPIError(requested)
			case Warn:
				logger.Warnf("Platform requested deprecated API '%s'", requested)
			default:
				logger.Warnf("Platform requested deprecated API '%s'", requested)
			}
		}
		if apis.IsExperimental(requestedAPI) {
			switch ExperimentalAPIs {
			case Quiet:
				break
			case Error:
				logger.Errorf("Platform requested experimental API '%s'", requested)
				logger.Errorf("Experimental APIs are disabled by %s=%s", EnvExperimentalAPIs, Error)
				return platformAPIError(requested)
			case Warn:
				logger.Warnf("Platform requested experimental API '%s'", requested)
			default:
				logger.Warnf("Platform requested experimental API '%s'", requested)
			}
		}
		return nil
	}
	return platformAPIError(requested)
}

func platformAPIError(requested string) error {
	return fmt.Errorf("platform API version '%s' is incompatible with the lifecycle", requested)
}

func VerifyBuildpackAPI(bp string, requested string, apis api.APIs, logger log.Logger) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return fmt.Errorf("failed to parse buildpack API '%s' for buildpack '%s'", requestedAPI, bp)
	}
	if apis.IsSupported(requestedAPI) {
		if apis.IsDeprecated(requestedAPI) {
			switch Deprecation {
			case Quiet:
				break
			case Error:
				logger.Errorf("Buildpack '%s' requests deprecated API '%s'", bp, requested)
				logger.Errorf("Deprecated APIs are disabled by %s=%s", EnvDeprecation, Error)
				return buildpackAPIError(bp, requested)
			case Warn:
				logger.Warnf("Buildpack '%s' requests deprecated API '%s'", bp, requested)
			default:
				logger.Warnf("Buildpack '%s' requests deprecated API '%s'", bp, requested)
			}
		}
		if apis.IsExperimental(requestedAPI) {
			switch ExperimentalAPIs {
			case Quiet:
				break
			case Error:
				logger.Errorf("Buildpack '%s' requests experimental API '%s'", bp, requested)
				logger.Errorf("Experimental APIs are disabled by %s=%s", EnvExperimentalAPIs, Error)
				return buildpackAPIError(bp, requested)
			case Warn:
				logger.Warnf("Buildpack '%s' requests experimental API '%s'", bp, requested)
			default:
				logger.Warnf("Buildpack '%s' requests experimental API '%s'", bp, requested)
			}
		}
		return nil
	}
	return buildpackAPIError(bp, requested)
}

func buildpackAPIError(bp string, requested string) error {
	return fmt.Errorf("buildpack '%s' requests buildpack API version '%s' which is incompatible with the lifecycle", bp, requested)
}
