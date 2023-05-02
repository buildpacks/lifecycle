package env

// # Builder Image

// A "builder" image contains a build-time base image, buildpacks, a lifecycle, and configuration files.

const (
	// VarBuildConfigDir is the location of the build config directory,
	// which may specify environment variables for buildpacks and extensions.
	VarBuildConfigDir = "CNB_BUILD_CONFIG_DIR"

	// VarBuildpacksDir is the location of the directory containing buildpacks.
	VarBuildpacksDir = "CNB_BUILDPACKS_DIR"

	// VarExtensionsDir is the location of the directory containing extensions.
	VarExtensionsDir = "CNB_EXTENSIONS_DIR"

	// VarOrderPath is the location of the order file, which is used for detection.
	// It contains a list of one or more buildpack groups
	// to be tested against application source code, so that the appropriate group for a given build can be determined.
	VarOrderPath = "CNB_ORDER_PATH"

	// VarRunPath is the location of the run file, which contains information about the runtime base image.
	VarRunPath = "CNB_RUN_PATH"

	// VarStackPath is the location of the (deprecated) stack file, which contains information about the runtime base image.
	VarStackPath = "CNB_STACK_PATH"
)
