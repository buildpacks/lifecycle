package acceptance

const (
	dockerfileName = "Dockerfile.windows"
	exe            = ".exe"
	execDBpDir     = "0.6_buildpack"
	rootDir        = `c:\`
)

var dockerSocketMount = []string{} // Not used in Windows tests
