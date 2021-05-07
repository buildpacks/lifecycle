package v07

import (
	"github.com/buildpacks/lifecycle/cmd"
	v06 "github.com/buildpacks/lifecycle/platform/v06"
)

type Platform struct {
	api string
}

func NewPlatform(apiStr string) *Platform {
	return &Platform{api: apiStr}
}

func (p *Platform) SupportsAssetPackages() bool {
	return true
}

func (p *Platform) API() string {
	return p.api
}
func (p *Platform) CodeFor(errType cmd.LifecycleExitError) int {
	// uses same exit codes as platform version v06
	if code, ok := v06.ExitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
