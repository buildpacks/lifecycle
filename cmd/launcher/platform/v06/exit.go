package v06

import (
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/cmd/launcher/platform/common"
)

var exitCodes = map[common.LifecycleExitError]int{
	// launch phase errors: 80-89
	common.LaunchError: 82, // LaunchError indicates generic launch error
}

func (p *v06Platform) CodeFor(errType common.LifecycleExitError) int {
	return CodeFor(errType)
}

func CodeFor(errType common.LifecycleExitError) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
