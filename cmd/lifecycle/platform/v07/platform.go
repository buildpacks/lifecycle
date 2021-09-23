package v07

import (
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform/common"
)

type v07Platform struct{}

func NewPlatform() common.Platform {
	return &v07Platform{}
}

func (p *v07Platform) API() string {
	return "0.7"
}
