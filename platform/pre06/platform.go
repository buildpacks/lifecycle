package pre06

import "github.com/buildpacks/lifecycle/api"

type Platform struct {
	api *api.Version
}

func NewPlatform(apiString string) *Platform {
	return &Platform{api: api.MustParse(apiString)}
}

func (p *Platform) API() string {
	return p.api.String()
}

func (p *Platform) SupportsAssetPackages() bool {
	return false
}
