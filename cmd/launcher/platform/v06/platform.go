package v06

import (
	"github.com/buildpacks/lifecycle/cmd/launcher/platform/common"
)

type v06Platform struct{}

func NewPlatform() common.Platform {
	return &v06Platform{}
}

func (p *v06Platform) API() string {
	return "0.6"
}
