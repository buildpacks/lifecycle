package v07

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform/common"
)

type Platform struct {
	api *api.Version
	common.Platform
}

func NewPlatform(prevPlatform common.Platform) *Platform {
	return &Platform{
		api:      api.MustParse("0.7"),
		Platform: prevPlatform,
	}
}

func (p *Platform) API() string {
	return p.api.String()
}

func (p *Platform) SupportsAssetPackages() bool {
	return true
}
