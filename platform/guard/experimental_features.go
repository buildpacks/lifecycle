package guard

import (
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/log"
)

var (
	ExperimentalFeaturesMode = envOrDefault(EnvExperimentalFeatures, DefaultExperimentalFeatures)
)

const (
	EnvExperimentalFeatures = "CNB_PLATFORM_EXPERIMENTAL_MODE"

	DefaultExperimentalFeatures = ModeWarn
)

func ExperimentalFeature(feature string, logger log.Logger) error {
	switch ExperimentalFeaturesMode {
	case ModeQuiet:
		break
	case ModeError:
		logger.Errorf("Experimental feature '%s' requested", feature)
		return errors.Errorf("Experimental features are disabled by %s=%s", EnvExperimentalFeatures, ExperimentalFeaturesMode)
	case ModeWarn:
		logger.Warnf("Experimental feature '%s' requested", feature)
	default:
		logger.Warnf("Experimental feature '%s' requested", feature)
	}
	return nil
}
