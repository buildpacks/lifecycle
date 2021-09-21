package v07

import (
	"github.com/buildpacks/lifecycle/cmd/launcher/platform/common"
)

func (p *v07Platform) CodeFor(errType common.LifecycleExitError) int {
	return p.previousPlatform.CodeFor(errType)
}
