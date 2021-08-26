//+build linux darwin

package acceptance

import "path"

const (
	exe                    = ""
	execDBpDir             = "0.5_buildpack"
	containerBaseImage     = "busybox"
	containerBaseImageFull = "ubuntu:bionic"
)

var dockerSocketMount = []string{
	"--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock",
	"--user", "0",
}

func ctrPath(unixPathParts ...string) string {
	return path.Join(unixPathParts...)
}
