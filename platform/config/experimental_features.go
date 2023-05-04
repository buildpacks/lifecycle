package config

import (
	"fmt"

	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/env"
)

const (
	FeatureDockerfiles  = "Dockerfiles"
	FeatureLayoutFormat = "export to OCI layout format"
)

var ExperimentalMode = envOrDefault(env.VarExperimentalMode, ModeWarn)

func VerifyExperimental(requested string, logger log.Logger) error {
	switch ExperimentalMode {
	case ModeQuiet:
		break
	case ModeError:
		logger.Errorf("Platform requested experimental feature '%s'", requested)
		return fmt.Errorf("experimental features are disabled by %s=%s", env.VarExperimentalMode, ModeError)
	case ModeWarn:
		logger.Warnf("Platform requested experimental feature '%s'", requested)
	default:
		// This shouldn't be reached, as ExperimentalMode is always set.
		logger.Warnf("Platform requested experimental feature '%s'", requested)
	}
	return nil
}
