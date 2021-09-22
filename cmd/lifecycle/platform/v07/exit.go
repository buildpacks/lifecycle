package v07

import (
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform/common"
	v06 "github.com/buildpacks/lifecycle/cmd/lifecycle/platform/v06"
)

func (p *v07Platform) CodeFor(errType common.LifecycleExitError) int {
	return v06.CodeFor(errType)
}
