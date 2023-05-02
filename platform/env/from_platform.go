package env

// # Platform Inputs

// ## Operator Experience

const (
	// VarPlatformAPI is the requested Platform API version.
	// The CNB Platform Interface Specification, also known as the Platform API, defines the contract between a platform and the lifecycle.
	// Multiple Platform API versions are supported by the lifecycle; for the list of supported versions, see [apiVersion.Platform].
	// This gives platform operators the flexibility to upgrade to newer lifecycle versions without breaking existing platform implementations,
	// as long as the Platform API version in use is still supported by the CNB project.
	// To view the Platform Interface Specification, see https://github.com/buildpacks/spec/blob/main/platform.md for the latest supported version,
	// or https://github.com/buildpacks/spec/blob/platform/<version>/platform.md for a specific version.
	// The Platform API version can be configured on a per-build basis through the environment; if no version is specified,
	// the default version is used.
	VarPlatformAPI = "CNB_PLATFORM_API"

	// VarLogLevel is the requested log level.
	VarLogLevel = "CNB_LOG_LEVEL"

	// VarNoColor if false instructs the lifecycle to print color to the terminal.
	VarNoColor = "CNB_NO_COLOR"

	// VarDeprecationMode is the desired behavior when deprecated APIs (either Platform or Buildpack) are requested.
	VarDeprecationMode = "CNB_DEPRECATION_MODE"

	// VarExperimentalMode is the desired behavior when experimental features (such as export to OCI layout format, or builds with image extensions) are requested.
	VarExperimentalMode = "CNB_EXPERIMENTAL_MODE"
)

// ## Build Inputs

// The following are directory locations that are inputs to the `detect` and `build` phases. They are passed through to buildpacks and/or extensions by the lifecycle,
// and will each typically be a separate volume mount.
const (
	VarAppDir      = "CNB_APP_DIR"
	VarLayersDir   = "CNB_LAYERS_DIR"
	VarPlatformDir = "CNB_PLATFORM_DIR"
)

// The following instruct the lifecycle where to write files and data during the build.
const (
	// VarAnalyzedPath is the location of the analyzed file, an output of the `analyze` phase.
	// It contains digest references to OCI images and metadata that are needed for the build.
	// It is an input to (and may be modified by) later lifecycle phases.
	VarAnalyzedPath = "CNB_ANALYZED_PATH"

	// VarExtendedDir is the location of the directory where the lifecycle should copy any image layers
	// created from applying generated Dockerfiles to a build- or run-time base image.
	VarExtendedDir = "CNB_EXTENDED_DIR"

	// VarGeneratedDir is the location of the directory where the lifecycle should copy any Dockerfiles
	// output by image extensions during the `generate` phase.
	VarGeneratedDir = "CNB_GENERATED_DIR"

	// VarGroupPath is the location of the group file, an output of the `detect` phase.
	// It contains the group of buildpacks that detected.
	VarGroupPath = "CNB_GROUP_PATH"

	// VarPlanPath is the location of the plan file, an output of the `detect` phase.
	// It contains information about dependencies that are needed for the build.
	VarPlanPath = "CNB_PLAN_PATH"

	// VarReportPath is the location of the report file, an output of the `export` phase.
	// It contains information about the output application image.
	VarReportPath = "CNB_REPORT_PATH"
)

// The following are images used by the lifecycle during the build.
const (
	// VarBuildImage is a reference to the build-time base image. It is needed when image extensions are used to extend the build-time base image.
	VarBuildImage = "CNB_BUILD_IMAGE"

	// VarPreviousImage is a reference to a previously built image; if not provided, it defaults to the output image reference.
	// It allows the lifecycle to re-use image layers that are unchanged from the previous build, avoiding the re-uploading
	// of data to the registry or daemon.
	VarPreviousImage = "CNB_PREVIOUS_IMAGE"

	// VarRunImage is a reference to the runtime base image. It is used to construct the output application image.
	VarRunImage = "CNB_RUN_IMAGE"
)

// ## Caching

const (
	// VarCacheDir is the location of the cache directory. Only one of cache directory or cache image may be used.
	// The cache is used to store buildpack-generated layers that are needed at build-time for future builds.
	VarCacheDir = "CNB_CACHE_DIR"

	// VarCacheImage is a reference to the cache image in an OCI registry. Only one of cache directory or cache image may be used.
	// The cache is used to store buildpack-generated layers that are needed at build-time for future builds.
	// Cache images in a daemon are disallowed (for performance reasons).
	VarCacheImage = "CNB_CACHE_IMAGE"

	// VarLaunchCacheDir is the location of the launch cache directory.
	// The launch cache is used when exporting to a daemon to store buildpack-generated layers, in order to speed up data retrieval for future builds.
	VarLaunchCacheDir = "CNB_LAUNCH_CACHE_DIR"

	// VarSkipLayers when true will instruct the lifecycle to ignore layers from a previously built image.
	VarSkipLayers = "CNB_SKIP_LAYERS"

	// VarSkipRestore is used when running the creator, and is equivalent to passing SkipLayers to both the analyzer and
	// the restorer in the 5-phase invocation.
	VarSkipRestore = "CNB_SKIP_RESTORE"

	// VarKanikoCacheTTL is the amount of time to persist layers cached by kaniko during the `extend` phase.
	VarKanikoCacheTTL = "CNB_KANIKO_CACHE_TTL"
)

// ## Build Outputs

// ### Export Target

// The default export target for the application image is an OCI registry.

const (
	// VarRegistryAuth contains JSON-encoded registry credentials.
	// When exporting to an OCI registry, registry credentials must be provided either on-disk (e.g., `~/.docker/config.json`),
	// via a credential helper, or via VarRegistryAuth. See [auth.DefaultKeychain] for further information.
	VarRegistryAuth = "CNB_REGISTRY_AUTH"

	// VarUseDaemon if true configures the lifecycle to export the application image to a daemon satisfying the Docker socket interface (e.g., docker, podman).
	// When exporting to a daemon, the socket must be available in the build environment and the lifecycle must be run as root.
	VarUseDaemon = "CNB_USE_DAEMON"

	// VarUseLayout if true configures the lifecycle to export the application image to OCI layout format on disk.
	VarUseLayout = "CNB_USE_LAYOUT"

	// VarLayoutDir must be set when exporting to OCI layout format to configure the root directory where OCI layout images will be saved.
	// Additionally, the lifecycle will read input images such as `run-image` and `previous-image` from this directory.
	VarLayoutDir = "CNB_LAYOUT_DIR"
)

// ### Application Image

// The following are configuration options for the output application image.
const (
	// VarProcessType is the default process for the application image, the entrypoint in the output image config.
	VarProcessType = "CNB_PROCESS_TYPE"

	// VarProjectMetadataPath is the location of the project metadata file. It contains information about the source repository
	// that is added as metadata to the application image.
	VarProjectMetadataPath = "CNB_PROJECT_METADATA_PATH"
)

// ## Image Extensions

const (
	// VarExtendKind is the kind of base image to extend (build or run) when running the extender.
	VarExtendKind = "CNB_EXTEND_KIND"
)

// ## Rebase

const (
	// VarForceRebase if true will force the rebaser to rebase the application image even if the operation is unsafe.
	VarForceRebase = "CNB_FORCE_REBASE"
)
