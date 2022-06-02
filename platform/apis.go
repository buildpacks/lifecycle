package platform

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/guard"
)

var (
	APIs = api.NewAPIsMustParse([]string{"0.3", "0.4", "0.5", "0.6", "0.7", "0.8", "0.9"}, nil)
)

func GuardAPI(api string, logger log.Logger) error {
	return guard.PlatformAPI(api, APIs, logger)
}
