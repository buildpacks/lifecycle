package acceptance

import (
	"path"
	"strings"
)

type VariableHelper struct {
	OS string
}

func (h *VariableHelper) DummyCommand() []string {
	if h.OS == "windows" {
		return []string{"cmd", "/c", "exit 0"}
	}
	return []string{"true"}
}

func (h *VariableHelper) DockerSocketMount() []string {
	if h.OS == "windows" {
		return []string{
			"--mount", `type=npipe,source=\\.\pipe\docker_engine,target=\\.\pipe\docker_engine`,
			"--user", "ContainerAdministrator",
		}
	}
	return []string{
		"--mount", "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock",
		"--user", "0",
	}
}

func (h *VariableHelper) ContainerBaseImage() string {
	if h.OS == "windows" {
		return "mcr.microsoft.com/windows/nanoserver:1809"
	}
	return "ubuntu:bionic"
}

func (h *VariableHelper) VolumeHelperImage() string {
	if h.OS == "windows" {
		return "mcr.microsoft.com/windows/nanoserver:1809"
	}
	return "busybox"
}

func (h *VariableHelper) CtrPath(unixPathParts ...string) string {
	unixPath := path.Join(unixPathParts...)
	if h.OS == "windows" {
		windowsPath := strings.ReplaceAll(unixPath, "/", `\`)
		if strings.HasPrefix(windowsPath, `\`) {
			return "c:" + windowsPath
		}
		return windowsPath
	}
	return unixPath
}

func (h *VariableHelper) Dockerfilename() string {
	if h.OS == "windows" {
		return "Dockerfile.windows"
	}
	return "Dockerfile"
}

func (h *VariableHelper) ExecDBpDir() string {
	if h.OS == "windows" {
		return "0.6_buildpack"
	}
	return "0.5_buildpack"
}

func (h *VariableHelper) Exe() string {
	if h.OS == "windows" {
		return ".exe"
	}
	return ""
}
