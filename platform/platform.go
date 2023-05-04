package platform

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/config"
	"github.com/buildpacks/lifecycle/platform/env"
	"github.com/buildpacks/lifecycle/platform/exit"
)

type LifecyclePhase int

const (
	Analyze LifecyclePhase = iota
	Detect
	Restore
	Extend
	Build
	Export
	Create
	Rebase
)

// Platform holds lifecycle inputs and outputs for a given Platform API version and lifecycle phase.
type Platform struct {
	*LifecycleInputs
	exit.Exiter
}

// New returns a Platform from the Platform API version requested by `env.VarPlatformAPI`
// with default lifecycle inputs and an exiter service,
// or an error if the requested Platform API version is unsupported.
func New(logger log.Logger) (*Platform, error) {
	platformAPI := envOrDefault(env.PlatformAPI, DefaultPlatformAPI)
	if err := config.VerifyPlatformAPI(platformAPI, logger); err != nil {
		return nil, err
	}
	return &Platform{
		LifecycleInputs: NewLifecycleInputs(api.MustParse(platformAPI)),
		Exiter:          exit.NewExiter(platformAPI),
	}, nil
}

func (p *Platform) API() *api.Version {
	return p.PlatformAPI
}
