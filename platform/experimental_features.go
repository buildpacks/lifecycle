package platform

import (
	"errors"
	"os"

	"github.com/buildpacks/lifecycle/log"
)

const (
	EnvExperimentalMode = "CNB_EXPERIMENTAL_MODE"

	DefaultExperimentalMode = ExperimentalModeError

	ExperimentalModeQuiet = "quiet"
	ExperimentalModeWarn  = "warn"
	ExperimentalModeError = "error"

	FeatureDockerfiles = "Dockerfiles"
)

var ExperimentalMode = envOrDefault(EnvExperimentalMode, DefaultExperimentalMode)

func GuardExperimental(requested string, logger log.Logger) error {
	switch ExperimentalMode {
	case ExperimentalModeQuiet:
		break
	case ExperimentalModeError:
		logger.Errorf("Platform requested experimental feature '%s'", requested)
		logger.Errorf("Experimental features are disabled by %s=%s", EnvExperimentalMode, ExperimentalModeError)
		return errors.New("experimental feature")
	case ExperimentalModeWarn:
		logger.Warnf("Platform requested experimental feature '%s'", requested)
	default:
		logger.Warnf("Platform requested experimental feature '%s'", requested)
	}
	return nil
}

func envOrDefault(key string, defaultVal string) string {
	if envVal := os.Getenv(key); envVal != "" {
		return envVal
	}
	return defaultVal
}
