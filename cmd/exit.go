package cmd

import (
	"os"

	"github.com/buildpacks/lifecycle/platform/exit"
)

func Exit(err error) {
	if err == nil {
		os.Exit(0)
	}
	DefaultLogger.Errorf("%s\n", err)
	if err, ok := err.(*exit.Error); ok {
		os.Exit(err.Code)
	}
	os.Exit(exit.CodeForFailed)
}

func ExitWithVersion() {
	DefaultLogger.Infof(buildVersion())
	os.Exit(0)
}
