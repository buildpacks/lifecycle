package platform

import (
	"github.com/buildpacks/lifecycle/api"
	v05 "github.com/buildpacks/lifecycle/platform/v05"
	v06 "github.com/buildpacks/lifecycle/platform/v06"
)

type Platform interface {
	API() string
	CodeFor(errType string) int
}

func NewPlatform(apiStr string) Platform {
	var platform Platform
	platformAPI := api.MustParse(apiStr)
	if platformAPI.Compare(api.MustParse("0.5")) > 0 { // platform API > 0.5
		platform = v06.NewPlatform(apiStr)
	} else {
		platform = v05.NewPlatform(apiStr)
	}
	return platform
}
