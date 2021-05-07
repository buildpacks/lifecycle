package platform

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	v05 "github.com/buildpacks/lifecycle/platform/v05"
	v06 "github.com/buildpacks/lifecycle/platform/v06"
	v07 "github.com/buildpacks/lifecycle/platform/v07"
)

type Platform interface {
	API() string
	CodeFor(errType cmd.LifecycleExitError) int
	SupportsAssetPackages() bool
}

func NewPlatform(apiStr string) Platform {
	platformAPI := api.MustParse(apiStr)
	switch {
	case platformAPI.Compare(api.MustParse("0.5")) <= 0: // platform API < 0.6
		return v05.NewPlatform(apiStr)
	case platformAPI.Compare(api.MustParse("0.6")) <= 0: // platform API 0.6
		return v06.NewPlatform(apiStr)
	default: // platform API 0.7
		return v07.NewPlatform(apiStr)
	}
}
