package platform

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/modes"
)

var APIs = api.NewAPIsMustParse([]string{"0.3", "0.4", "0.5", "0.6", "0.7", "0.8", "0.9"}, nil)

func VerifyAPI(requested string, logger Logger) error {
	return modes.VerifyPlatformAPI(requested, APIs, logger)
}
