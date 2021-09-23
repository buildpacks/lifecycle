package legacy

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd/launcher/platform/common"
)

type pre06Platform struct {
	api *api.Version
}

func NewPlatform(apiString string) common.Platform {
	return &pre06Platform{
		api: api.MustParse(apiString),
	}
}

func (p *pre06Platform) API() string {
	return p.api.String()
}