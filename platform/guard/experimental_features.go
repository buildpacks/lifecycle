package guard

import (
	"fmt"

	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/env"
)

const (
	FeatureDockerfiles = "Dockerfiles"
	LayoutFormat       = "export to OCI layout format"
)

var ExperimentalMode = EnvOrDefault(env.VarExperimentalMode, platform.DefaultExperimentalMode)

func GuardExperimental(requested string, logger log.Logger) error {
	switch ExperimentalMode {
	case platform.ModeQuiet:
		break
	case platform.ModeError:
		logger.Errorf("Platform requested experimental feature '%s'", requested)
		return fmt.Errorf("experimental features are disabled by %s=%s", env.VarExperimentalMode, platform.ModeError)
	case platform.ModeWarn:
		logger.Warnf("Platform requested experimental feature '%s'", requested)
	default:
		// This shouldn't be reached, as ExperimentalMode is always set.
		logger.Warnf("Platform requested experimental feature '%s'", requested)
	}
	return nil
}
