package v06

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
		api:              api.MustParse("0.6"),
		previousPlatform: previousPlatform,
	}
}

func (p *Platform) API() string {
	return p.api.String()
}

func (p *Platform) SupportsAssetPackages() bool {
	return false
}

func (p *Platform) SupportsMixinValidation() bool {
	return false
}
