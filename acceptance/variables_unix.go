//go:build linux || darwin

package acceptance

import "path"

const (
	containerBaseImage     = "busybox"
	containerBaseImageFull = "ubuntu:bionic"
	dockerfileName         = "Dockerfile"
	exe                    = ""
	execDBpDir             = "0.9_buildpack"
)

var dockerSocketMount = []string{
	"--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock",
	"--user", "0",
}

func ctrPath(unixPathParts ...string) string {
	return path.Join(unixPathParts...)
}
