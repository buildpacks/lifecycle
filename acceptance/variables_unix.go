//+build linux darwin

package acceptance

const (
	dockerfileName = "Dockerfile"
	exe            = ""
	execDBpDir     = "0.5_buildpack"
	rootDir        = "/"
)

var dockerSocketMount = []string{
	"--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock",
	"--user", "0",
}
