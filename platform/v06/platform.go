package v06

import v05 "github.com/buildpacks/lifecycle/platform/v05"

type Platform struct {
	*v05.Platform
}

func NewPlatform(apiStr string) *Platform {
	return &Platform{
		Platform: v05.NewPlatform(apiStr),
	}
}
