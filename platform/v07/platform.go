package v07

import v06 "github.com/buildpacks/lifecycle/platform/v06"

type Platform struct {
	*v06.Platform
}

func NewPlatform(apiStr string) *Platform {
	return &Platform{
		Platform: v06.NewPlatform(apiStr),
	}
}

func (p *Platform) SupportsAssetPackages() bool {
	return true
}
