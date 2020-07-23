package variables

const (
	ContainerBaseImage = "ubuntu:bionic"
	DockerfileName     = "Dockerfile"
	VolumeHelperImage  = "busybox"
)

var DummyCommand = []string{"true"}

var DockerSocketMount = []string{
	"--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock",
	"--user", "0",
}
