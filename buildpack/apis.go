package buildpack

import (
	"github.com/buildpacks/lifecycle/api"
)

var (
	APIs = api.NewAPIsMustParse([]string{"0.2", "0.3", "0.4", "0.5", "0.6", "0.7", "0.8"}, nil)
)
