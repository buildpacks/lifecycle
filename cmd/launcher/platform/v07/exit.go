package v07

import (
	"github.com/buildpacks/lifecycle/cmd/launcher/platform/common"
)

var exitCodes = map[common.LifecycleExitError]int{
	// launch phase errors: 80-89
	common.LaunchError: 82, // LaunchError indicates generic launch error
}

func (p *v07Platform) CodeFor(errType common.LifecycleExitError) int {
	return common.CodeFor(errType, exitCodes)
}
