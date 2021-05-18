package platform

import (
	"fmt"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform/common"
	v05 "github.com/buildpacks/lifecycle/platform/v05"
	v06 "github.com/buildpacks/lifecycle/platform/v06"
	v07 "github.com/buildpacks/lifecycle/platform/v07"
)

var platform05 = v05.NewPlatform()
var platform06 = v06.NewPlatform(platform05)
var platform07 = v07.NewPlatform(platform06)

var platformMap = map[string]common.Platform {
	"0.5": platform05,
	"0.6": platform06,
	"0.7": platform07,
}

func NewPlatform(apiStr string) (common.Platform, error) {
	platformAPI := api.MustParse(apiStr)
	p, ok := platformMap[platformAPI.String()]
	switch {
	case platformAPI.Compare(api.MustParse("0.5")) < 0:
		return platform05, nil
	case !ok:
		return nil, fmt.Errorf("unable to create platform for api %s: unknown api", apiStr)
	default:
		return p, nil
	}
}
