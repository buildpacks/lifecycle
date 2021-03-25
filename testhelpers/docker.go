package testhelpers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/pkg/errors"
)

var (
	dockerCliOnce sync.Once
	dockerCliVal  dockercli.CommonAPIClient
)

func DockerCli(t *testing.T) dockercli.CommonAPIClient {
	dockerCliOnce.Do(func() {
		var dockerCliErr error
		dockerCliVal, dockerCliErr = dockercli.NewClientWithOpts(dockercli.FromEnv, dockercli.WithVersion("1.38"))
		AssertNil(t, dockerCliErr)
	})
	return dockerCliVal
}

func DockerBuild(t *testing.T, name, context string, ops ...DockerCmdOp) {
	t.Helper()
	args := formatArgs([]string{"-t", name, context}, ops...)
	Run(t, exec.Command("docker", append([]string{"build"}, args...)...)) // #nosec G204
}

func DockerImageRemove(t *testing.T, name string) {
	t.Helper()
	Run(t, exec.Command("docker", "rmi", name))
}

func DockerRun(t *testing.T, image string, ops ...DockerCmdOp) string {
	t.Helper()
	args := formatArgs([]string{image}, ops...)
	return Run(t, exec.Command("docker", append([]string{"run", "--rm"}, args...)...)) // #nosec G204
}

//DockerRunAndCopy runs a container and once stopped, outputCtrPath is copied to outputDir
func DockerRunAndCopy(t *testing.T, containerName, outputDir, outputCtrPath, image string, ops ...DockerCmdOp) string {
	ops = append(ops, WithFlags("--name", containerName))
	args := formatArgs([]string{image}, ops...)

	output := Run(t, exec.Command("docker", append([]string{"run"}, args...)...))     // #nosec G204
	Run(t, exec.Command("docker", "cp", containerName+":"+outputCtrPath, outputDir))  // #nosec G204
	return output
}

//DockerSeedRunAndCopy copies srcDir to container's srcCtrPath before container is started. Once stopped, outputCtrPath is copied to outputDir
//On WCOW, only works when seeding to container directory (not a mounted volume)
func DockerSeedRunAndCopy(t *testing.T, containerName, srcDir, srcCtrPath, outputDir, outputCtrPath, image string, ops ...DockerCmdOp) string {
	ops = append(ops, WithFlags("--name", containerName))
	args := formatArgs([]string{image}, ops...)

	output := Run(t, exec.Command("docker", append([]string{"create"}, args...)...))
	output += Run(t, exec.Command("docker", "cp", srcDir, containerName+":"+srcCtrPath))
	output += Run(t, exec.Command("docker", "start", "--attach", containerName))
	output += Run(t, exec.Command("docker", "cp", containerName+":"+outputCtrPath, outputDir))

	return output
}

func DockerCopyOut(t *testing.T, containerName, srcCtrPath, outputDir string) string {
	return Run(t, exec.Command("docker", "cp", containerName+":"+srcCtrPath, outputDir))
}

func DockerContainerExists(t *testing.T, containerName string) bool {
	output := Run(t, exec.Command("docker", "ps", "-a"))
	return strings.Contains(output, containerName)
}

func DockerVolumeRemove(t *testing.T, volume string) {
	Run(t, exec.Command("docker", "volume", "rm", volume))
}

func DockerVolumeExists(t *testing.T, volumeName string) bool {
	if volumeName == "" {
		return false
	}
	output := Run(t, exec.Command("docker", "volume", "ls"))
	return strings.Contains(output, volumeName)
}

// TODO: re-work this function to exec the docker cli, or convert other docker helpers to using the client library.
func PushImage(dockerCli dockercli.CommonAPIClient, ref string, auth string) error {
	rc, err := dockerCli.ImagePush(context.Background(), ref, dockertypes.ImagePushOptions{RegistryAuth: auth})
	if err != nil {
		return errors.Wrap(err, "pushing image")
	}

	defer rc.Close()
	err = checkResponse(rc)
	if err != nil {
		return errors.Wrap(err, "push response")
	}

	return nil
}

//SeedDockerVolume only works with Linux daemons as Windows only mounts volumes for started containers
func SeedDockerVolume(t *testing.T, srcPath string) string {
	volumeName := "test-volume-" + RandString(10)
	containerName := "test-volume-helper-" + RandString(10)
	volumeHelperImage := "alpine"

	Run(t, exec.Command("docker", "pull", volumeHelperImage))
	Run(t, exec.Command("docker", append([]string{
		"run",
		"--volume", volumeName + ":" + "/target", // create a new empty volume
		"--name", containerName,
		volumeHelperImage},
		"true")...)) // #nosec G204
	defer Run(t, exec.Command("docker", "rm", containerName))

	fis, err := ioutil.ReadDir(srcPath)
	AssertNil(t, err)
	for _, fi := range fis {
		Run(t, exec.Command(
			"docker", "cp",
			filepath.Join(srcPath, fi.Name()),
			containerName+":"+"/target",
		)) // #nosec G204
	}

	return volumeName
}

func checkResponse(responseBody io.Reader) error {
	body, err := ioutil.ReadAll(responseBody)
	if err != nil {
		return errors.Wrap(err, "reading body")
	}

	messages := strings.Builder{}
	for _, line := range bytes.Split(body, []byte("\n")) {
		if len(line) == 0 {
			continue
		}

		var msg jsonmessage.JSONMessage
		err := json.Unmarshal(line, &msg)
		if err != nil {
			return errors.Wrapf(err, "expected JSON: %s", string(line))
		}

		if msg.Stream != "" {
			messages.WriteString(msg.Stream)
		}

		if msg.Error != nil {
			return errors.WithMessage(msg.Error, messages.String())
		}
	}

	return nil
}
