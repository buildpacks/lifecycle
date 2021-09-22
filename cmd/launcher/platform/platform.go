package platform

import (
	"fmt"

	"github.com/buildpacks/lifecycle/cmd/launcher/platform/common"
	"github.com/buildpacks/lifecycle/cmd/launcher/platform/pre06"
	v06 "github.com/buildpacks/lifecycle/cmd/launcher/platform/v06"
	v07 "github.com/buildpacks/lifecycle/cmd/launcher/platform/v07"
)

func NewPlatform(apiStr string) (common.Platform, error) {
	switch apiStr {
	case "0.3", "0.4", "0.5":
		return pre06.NewPlatform(apiStr), nil
	case "0.6":
		return v06.NewPlatform(), nil
	case "0.7":
		return v07.NewPlatform(), nil
	}
	return nil, fmt.Errorf("unable to create platform for api %s: unknown api", apiStr)
}
