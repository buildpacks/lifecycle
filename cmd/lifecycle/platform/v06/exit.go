package v06

import (
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform/common"
)

func (p *v06Platform) CodeFor(errType common.LifecycleExitError) int {
	return common.CodeFor(errType)
}
