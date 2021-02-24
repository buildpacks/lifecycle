//+build linux darwin

package acceptance

const (
	containerBaseImage = "ubuntu:bionic"
	dockerfileName     = "Dockerfile"
	exe                = ""
	rootDir            = "/"
)

var dockerSocketMount = []string{
	"--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock",
	"--user", "0",
}
