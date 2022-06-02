package guard

import (
	"fmt"
	"os"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/log"
)

var (
	DeprecationMode  = envOrDefault(EnvDeprecationMode, DefaultDeprecation)
	ExperimentalMode = envOrDefault(EnvExperimentalMode, DefaultExperimentalAPIs)
)

const (
	EnvDeprecationMode  = "CNB_DEPRECATION_MODE"
	EnvExperimentalMode = "CNB_EXPERIMENTAL_MODE"

	DefaultDeprecation      = ModeWarn
	DefaultExperimentalAPIs = ModeWarn

	ModeError = "error"
	ModeQuiet = "quiet"
	ModeWarn  = "warn"
)

func PlatformAPI(requested string, apis api.APIs, logger log.Logger) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return fmt.Errorf("failed to parse platform API '%s'", requested)
	}
	if apis.IsSupported(requestedAPI) {
		if apis.IsDeprecated(requestedAPI) {
			switch DeprecationMode {
			case ModeQuiet:
				break
			case ModeError:
				logger.Errorf("Platform requested deprecated API '%s'", requested)
				logger.Errorf("Deprecated APIs are disabled by %s=%s", EnvDeprecationMode, ModeError)
				return platformAPIError(requested)
			case ModeWarn:
				logger.Warnf("Platform requested deprecated API '%s'", requested)
			default:
				logger.Warnf("Platform requested deprecated API '%s'", requested)
			}
		}
		if apis.IsPrelease(requestedAPI) {
			switch ExperimentalMode {
			case ModeQuiet:
				break
			case ModeError:
				logger.Errorf("Platform requested experimental API '%s'", requested)
				logger.Errorf("Experimental APIs are disabled by %s=%s", EnvExperimentalMode, ModeError)
				return platformAPIError(requested)
			case ModeWarn:
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

func BuildpackAPI(bp string, requested string, apis api.APIs, logger log.Logger) error {
	requestedAPI, err := api.NewVersion(requested)
	if err != nil {
		return fmt.Errorf("failed to parse buildpack API '%s' for buildpack '%s'", requestedAPI, bp)
	}
	if apis.IsSupported(requestedAPI) {
		if apis.IsDeprecated(requestedAPI) {
			switch DeprecationMode {
			case ModeQuiet:
				break
			case ModeError:
				logger.Errorf("Buildpack '%s' requests deprecated API '%s'", bp, requested)
				logger.Errorf("Deprecated APIs are disabled by %s=%s", EnvDeprecationMode, ModeError)
				return buildpackAPIError(bp, requested)
			case ModeWarn:
				logger.Warnf("Buildpack '%s' requests deprecated API '%s'", bp, requested)
			default:
				logger.Warnf("Buildpack '%s' requests deprecated API '%s'", bp, requested)
			}
		}
		if apis.IsPrelease(requestedAPI) {
			switch ExperimentalMode {
			case ModeQuiet:
				break
			case ModeError:
				logger.Errorf("Buildpack '%s' requests experimental API '%s'", bp, requested)
				logger.Errorf("Experimental APIs are disabled by %s=%s", EnvExperimentalMode, ModeError)
				return buildpackAPIError(bp, requested)
			case ModeWarn:
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

func envOrDefault(key string, defaultVal string) string {
	if envVal := os.Getenv(key); envVal != "" {
		return envVal
	}
	return defaultVal
}
