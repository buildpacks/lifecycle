package cmd

import (
	"fmt"
	"os"

	"github.com/buildpacks/lifecycle/api"
)

func VerifyCompatibility() error {
	platformsAPI := os.Getenv("CNB_PLATFORM_API")
	if platformsAPI != "" {
		providedVersion, err := api.NewVersion(platformsAPI)
		if err != nil {
			return err
		}

		lcPlatformAPI := api.MustParse(PlatformAPI)
		if !lcPlatformAPI.SupportsVersion(providedVersion) {
			return FailErrCode(
				fmt.Errorf("the Lifecycle's Platform API version is %s which is incompatible with Platform API version %s", lcPlatformAPI.String(), platformsAPI),
				CodeIncompatible,
			)
		}
	}

	return nil
}
