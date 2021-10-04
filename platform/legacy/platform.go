package legacy

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform/common"
)

type legacyPlatform struct {
	api *api.Version
}

func NewPlatform(apiString string) common.Platform {
	return &legacyPlatform{
		api: api.MustParse(apiString),
	}
}

func (p *legacyPlatform) API() string {
	return p.api.String()
}
