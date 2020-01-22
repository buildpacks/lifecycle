package cmd

import (
	"fmt"
)

// The following variables are injected at compile time.
var (
	// Version is the version of the lifecycle and all produced binaries.
	Version = "0.0.0"
	// SCMCommit is the commit information provided by SCM.
	SCMCommit = ""
	// SCMRepository is the source repository.
	SCMRepository = ""
	// PlatformAPI is the version of the Platform API implemented.
	PlatformAPI = "0.0"
)

// buildVersion is a display format of the version and build metadata in compliance with semver.
func buildVersion() string {
	// noinspection GoBoolExpressions
	if SCMCommit == "" {
		return Version
	}

	return fmt.Sprintf("%s+%s", Version, SCMCommit)
}
