package platform

import (
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/internal/env"
)

const (
	EnvExperimentalModeFeatures     = "CNB_PLATFORM_EXPERIMENTAL_MODE"
	DefaultExperimentalModeFeatures = ModeWarn

	ModeError = "error"
	ModeQuiet = "quiet"
	ModeWarn  = "warn"
)

var (
	ExperimentalModeFeatures = env.OrDefault(EnvExperimentalModeFeatures, DefaultExperimentalModeFeatures)
)

func GuardExperimental(feature string, logger Logger) error {
	switch ExperimentalModeFeatures {
	case ModeQuiet:
		break
	case ModeError:
		logger.Errorf("Platform requested experimental feature '%s'", feature)
		return errors.Errorf("Experimental features are disabled by %s=%s", EnvExperimentalModeFeatures, ExperimentalModeFeatures)
	case ModeWarn:
		logger.Warnf("Platform requested experimental feature '%s'", feature)
	default:
		logger.Warnf("Platform requested experimental feature '%s'", feature)
	}
	return nil
}
