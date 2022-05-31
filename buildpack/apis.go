package buildpack

import (
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/log"
	"github.com/buildpacks/lifecycle/internal/modes"
)

var (
	APIs = api.NewAPIsMustParse([]string{"0.2", "0.3", "0.4", "0.5", "0.6", "0.7", "0.8"}, nil)
)

func VerifyAPI(bp string, requested string, logger log.Logger) error {
	return modes.VerifyBuildpackAPI(bp, requested, APIs, logger)
}
