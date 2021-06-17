package v07

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
)

type Platform struct {
	api              *api.Version
	previousPlatform platform.Platform
}

func NewPlatform(previousPlatform platform.Platform) *Platform {
	return &Platform{
		api:              api.MustParse("0.7"),
		previousPlatform: previousPlatform,
	}
}

func (p *Platform) API() string {
	return p.api.String()
}

func (p *Platform) SupportsAssetPackages() bool {
	return true
}

func (p *Platform) SupportsMixinValidation() bool {
	return true
}
