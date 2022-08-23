package kaniko

import (
	"path/filepath"
)

const kanikoDir = "/kaniko"

type DockerfileApplier struct {
	cacheDir               string
	cacheImageRef          string // an oci-layout image
	dockerfileBuildContext string
}

func NewDockerfileApplier() *DockerfileApplier {
	return &DockerfileApplier{
		cacheDir:               filepath.Join(kanikoDir, "cache", "base"),
		cacheImageRef:          filepath.Join("oci:", kanikoDir, "cache", "layers", "cached"),
		dockerfileBuildContext: "/workspace", // TODO (before merging): make configurable
	}
}
