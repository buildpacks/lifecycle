package common

type Platform interface {
	API() string
	CodeFor(errType LifecycleExitError) int
}
