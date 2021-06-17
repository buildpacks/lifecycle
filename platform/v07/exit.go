package v07

import (
	"github.com/buildpacks/lifecycle/platform"
)

func (p *Platform) CodeFor(errType platform.LifecycleExitError) int {
	return p.previousPlatform.CodeFor(errType)
}
