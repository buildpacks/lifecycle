package common

import "github.com/buildpacks/lifecycle"

type Platform interface {
	API() string
	CodeFor(errType LifecycleExitError) int
	AnalyzeOperations() []lifecycle.AnalyzeOperation
}
