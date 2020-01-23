package cmd

import (
	"fmt"
	"os"

	"github.com/buildpacks/lifecycle/api"
)

func VerifyCompatibility() error {
	platformsAPI := os.Getenv("CNB_PLATFORM_API")
	if platformsAPI != "" {
		platformAPIFromPlatform, err := api.NewVersion(platformsAPI)
		if err != nil {
			return err
		}

		platformAPIFromLifecycle := api.MustParse(PlatformAPI)
		if !api.IsPlatformAPICompatible(platformAPIFromLifecycle, platformAPIFromPlatform) {
			return FailErrCode(
				fmt.Errorf(
					"the Lifecycle's Platform API version is %s which is incompatible with Platform API version %s",
					platformAPIFromLifecycle.String(),
					platformAPIFromPlatform.String(),
				),
				CodeIncompatible,
			)
		}
	}

	return nil
}
