package modes

import (
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/internal/env"
	"github.com/buildpacks/lifecycle/internal/log"
)

const (
	EnvExperimentalFeatures     = "CNB_PLATFORM_EXPERIMENTAL_MODE"
	DefaultExperimentalFeatures = Warn
)

var (
	ExperimentalFeatures = env.OrDefault(EnvExperimentalFeatures, DefaultExperimentalFeatures)
)

func GuardExperimental(feature string, logger log.Logger) error {
	switch ExperimentalFeatures {
	case Quiet:
		break
	case Error:
		logger.Errorf("Experimental feature '%s' requested", feature)
		return errors.Errorf("Experimental features are disabled by %s=%s", EnvExperimentalFeatures, ExperimentalFeatures)
	case Warn:
		logger.Warnf("Experimental feature '%s' requested", feature)
	default:
		logger.Warnf("Experimental feature '%s' requested", feature)
	}
	return nil
}
