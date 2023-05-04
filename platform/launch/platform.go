package launch

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform/exit"
)

type Platform struct {
	exit.Exiter
	api *api.Version
}

func NewPlatform(apiStr string) *Platform {
	return &Platform{
		Exiter: exit.NewExiter(apiStr),
		api:    api.MustParse(apiStr),
	}
}

func (p *Platform) API() *api.Version {
	return p.api
}
