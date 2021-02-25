package acceptance

const (
	dockerfileName = "Dockerfile.windows"
	exe            = ".exe"
	rootDir        = `c:\`
)

var dockerSocketMount = []string{} // Not used in Windows tests
