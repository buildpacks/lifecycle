package platform

import "github.com/buildpacks/lifecycle/api"

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
	Phase LifecyclePhase
	LifecycleInputs
	Exiter
}

// NewPlatformFor accepts a lifecycle phase and Platform API version, and returns a Platform.
func NewPlatformFor(phase LifecyclePhase, platformAPI string) *Platform {
	var lifecycleInputs LifecycleInputs
	switch phase {
	case Analyze, Detect, Restore, Extend, Build, Export, Create, Rebase:
		lifecycleInputs = NewLifecycleInputs(api.MustParse(platformAPI))
	default:
		// nop
	}
	return &Platform{
		Phase:           phase,
		LifecycleInputs: lifecycleInputs,
		Exiter:          NewExiter(platformAPI),
	}
}

func (p *Platform) API() *api.Version {
	return p.PlatformAPI
}
