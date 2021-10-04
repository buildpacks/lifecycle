package v07

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform/common"
)

type v07Platform struct {
	api *api.Version
}

func NewPlatform() common.Platform {
	return &v07Platform{
		api: api.MustParse("0.7"),
	}
}

func (p *v07Platform) API() string {
	return p.api.String()
}
