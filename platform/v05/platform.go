package v05

import "github.com/buildpacks/lifecycle/api"

type Platform struct {
	api *api.Version
}

func NewPlatform() *Platform {
	return &Platform{api: api.MustParse("0.5")}
}

func (p *Platform) API() string {
	return p.api.String()
}

func (p *Platform) SupportsAssetPackages() bool {
	return false
}
