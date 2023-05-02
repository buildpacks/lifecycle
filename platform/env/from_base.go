package env

// # Base Image

// A build-time base image contains the OS-level dependencies needed for the build - i.e., dependencies needed for buildpack execution.

const (
	// DeprecatedVarStackID is the CNB stack id. It is passed through by the lifecycle to buildpacks.
	DeprecatedVarStackID = "CNB_STACK_ID"

	// VarUID is the user ID of the CNB user. It must match the UID of the user specified in the image config's `USER` field.
	VarUID = "CNB_USER_ID"

	// VarGID is the group ID of the CNB user. It must match the GID of the user specified in the image config's `USER` field.
	VarGID = "CNB_GROUP_ID"
)
