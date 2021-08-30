package acceptance

import (
	"path"
	"path/filepath"
	"strings"
)

const (
	containerBaseImage     = "mcr.microsoft.com/windows/nanoserver:1809"
	containerBaseImageFull = "mcr.microsoft.com/windows/nanoserver:1809"
	dockerfileName         = "Dockerfile.windows"
	exe                    = ".exe"
	execDBpDir             = "0.6_buildpack"
)

var dockerSocketMount = []string{
	"--mount", `type=npipe,source=\\.\pipe\docker_engine,target=\\.\pipe\docker_engine`,
	"--user", "ContainerAdministrator",
}

//ctrPath equivalent to path.Join but converts to Windows slashes and drive prefix when needed
func ctrPath(unixPathParts ...string) string {
	unixPath := path.Join(unixPathParts...)
	windowsPath := filepath.FromSlash(unixPath)
	if strings.HasPrefix(windowsPath, `\`) {
		return "c:" + windowsPath
	}
	return windowsPath
}
