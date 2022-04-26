package platform

import (
	"github.com/buildpacks/lifecycle/api"
)

type Platform struct {
	Exiter
	api *api.Version
}

func NewPlatform(apiStr string) *Platform {
	return &Platform{
		Exiter: NewExiter(apiStr),
		api:    api.MustParse(apiStr),
	}
}

func (p *Platform) API() *api.Version {
	return p.api
}
