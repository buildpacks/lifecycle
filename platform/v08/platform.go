package v08

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform/common"
)

type v08Platform struct {
	api *api.Version
}

func NewPlatform() common.Platform {
	return &v08Platform{
		api: api.MustParse("0.8"),
	}
}

func (p *v08Platform) API() string {
	return p.api.String()
}
