package common

const (
	LaunchError LifecycleExitError = iota // generic launch error
)

type LifecycleExitError int
