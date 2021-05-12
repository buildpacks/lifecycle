package lifecycle

import (
	"fmt"
	"os"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
)

func ValidateStack(stackMD platform.StackMetadata, runImage imgutil.Image) error {
	buildStackID, err := getBuildStack(stackMD)
	if err != nil {
		return err
	}

	runStackID, err := getRunStack(runImage)
	if err != nil {
		return err
	}

	if buildStackID != runStackID {
		return errors.New(fmt.Sprintf("incompatible stack: '%s' is not compatible with '%s'", runStackID, buildStackID))
	}
	return nil
}

func getRunStack(runImage imgutil.Image) (string, error) {
	runStackID, err := runImage.Label(platform.StackIDLabel)
	if err != nil {
		return "", errors.Wrap(err, "get run image label")
	}
	if runStackID == "" {
		return "", errors.New("get run image label: io.buildpacks.stack.id")
	}
	return runStackID, nil
}

func getBuildStack(stackMD platform.StackMetadata) (string, error) {
	var buildStackID string
	if buildStackID = os.Getenv(cmd.EnvStackID); buildStackID != "" {
		return buildStackID, nil
	}
	if buildStackID = stackMD.BuildImage.StackID; buildStackID != "" {
		return buildStackID, nil
	}
	return "", errors.New("CNB_STACK_ID is required when there is no stack metadata available")
}
