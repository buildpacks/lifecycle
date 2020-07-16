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

	"github.com/buildpacks/lifecycle/acceptance/variables"
)

var dockerCliVal dockercli.CommonAPIClient
var dockerCliOnce sync.Once

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
	Run(t, exec.Command("docker", append([]string{"build"}, args...)...))
}

func DockerImageRemove(t *testing.T, name string) {
	t.Helper()
	Run(t, exec.Command("docker", "rmi", name))
}

func DockerRun(t *testing.T, image string, ops ...DockerCmdOp) string {
	t.Helper()
	args := formatArgs([]string{image}, ops...)
	return Run(t, exec.Command("docker", append([]string{"run", "--rm"}, args...)...))
}

func DockerRunAndCopy(t *testing.T, image, path string, ops ...DockerCmdOp) (string, string) {
	containerName := "test-container-" + RandString(10)
	ops = append(ops, WithFlags("--name", containerName))
	args := formatArgs([]string{image}, ops...)

	output := Run(t, exec.Command("docker", append([]string{"run"}, args...)...))
	defer Run(t, exec.Command("docker", "rm", containerName))

	tempDir, err := ioutil.TempDir("", "test-docker-copy-")
	AssertNil(t, err)
	Run(t, exec.Command("docker", "cp", containerName+":"+path, tempDir))

	return output, tempDir
}

func DockerVolumeRemove(t *testing.T, volume string) {
	Run(t, exec.Command("docker", "volume", "rm", volume))
}

func ImageID(t *testing.T, repoName string) string {
	t.Helper()
	inspect, _, err := DockerCli(t).ImageInspectWithRaw(context.Background(), repoName)
	AssertNil(t, err)
	return inspect.ID
}

func PullImage(dockerCli *dockercli.Client, ref string) error {
	rc, err := dockerCli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{})
	if err != nil {
		// Retry
		rc, err = dockerCli.ImagePull(context.Background(), ref, dockertypes.ImagePullOptions{})
		if err != nil {
			return err
		}
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

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

func SeedDockerVolume(t *testing.T, srcPath string) string {
	volumeName := "test-volume-" + RandString(10)
	containerName := "test-volume-helper-" + RandString(10)

	Run(t, exec.Command("docker", "pull", variables.VolumeHelperImage))
	Run(t, exec.Command("docker", append([]string{
		"run",
		"--volume", volumeName + ":" + "/target", // create a new empty volume
		"--name", containerName,
		variables.VolumeHelperImage},
		variables.DummyCommand...)...))
	defer Run(t, exec.Command("docker", "rm", containerName))

	fis, err := ioutil.ReadDir(srcPath)
	AssertNil(t, err)
	for _, fi := range fis {
		Run(t, exec.Command(
			"docker", "cp",
			filepath.Join(srcPath, fi.Name()),
			containerName+":"+"/target",
		))
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
