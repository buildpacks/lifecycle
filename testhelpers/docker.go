package testhelpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

var dockerCliVal *dockercli.Client
var dockerCliOnce sync.Once

type DockerCmd struct {
	flags []string
	args  []string
}

type DockerCmdOp func(*DockerCmd)

func DockerCli(t *testing.T) *dockercli.Client {
	dockerCliOnce.Do(func() {
		var dockerCliErr error
		dockerCliVal, dockerCliErr = dockercli.NewClientWithOpts(dockercli.FromEnv, dockercli.WithVersion("1.38"))
		AssertNil(t, dockerCliErr)
	})
	return dockerCliVal
}

func DockerRun(t *testing.T, image string, ops ...DockerCmdOp) string {
	args := formatArgs(image, ops...)
	args = append([]string{"run", "--rm"}, args...) // prepend run --rm

	return Run(t, exec.Command("docker", args...))
}

func formatArgs(image string, ops ...DockerCmdOp) []string {
	cmd := DockerCmd{}

	for _, op := range ops {
		op(&cmd)
	}

	args := []string{image}
	args = append(cmd.flags, args...) // prepend flags
	args = append(args, cmd.args...)  // append args

	return args
}

func WithFlags(flags ...string) DockerCmdOp {
	return func(cmd *DockerCmd) {
		cmd.flags = append(cmd.flags, flags...)
	}
}

func WithArgs(args ...string) DockerCmdOp {
	return func(cmd *DockerCmd) {
		cmd.args = append(cmd.args, args...)
	}
}

func WithBash(args ...string) DockerCmdOp {
	return func(cmd *DockerCmd) {
		cmd.args = append([]string{"/bin/bash", "-c"}, args...)
	}
}

func DockerRunAndCopy(t *testing.T, image, path string, ops ...DockerCmdOp) (string, string) {
	containerName := "test-container-" + RandString(10)
	ops = append(ops, WithFlags("--name", containerName))
	args := formatArgs(image, ops...)
	args = append([]string{"run"}, args...) // prepend run

	output := Run(t, exec.Command("docker", args...))
	defer DockerContainerRemove(t, containerName)

	tempDir, err := ioutil.TempDir("", "test-docker-copy-")
	AssertNil(t, err)

	Run(t,
		exec.Command("docker", "cp", fmt.Sprintf("%s:%s", containerName, path), tempDir),
	)

	return output, tempDir
}

func DockerContainerRemove(t *testing.T, name string) {
	Run(t, exec.Command(
		"docker", "rm", name,
	))
}

func DockerVolumeRemove(t *testing.T, volume string) {
	Run(t, exec.Command(
		"docker", "volume", "rm", volume,
	))
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

func SeedDockerVolume(t *testing.T, srcPath, helperImage string) string { // TODO: make this a helper function that inspects the daemon info OS
	volumeName := "test-volume-" + RandString(10)
	containerName := "test-volume-helper-" + RandString(10)

	Run(t, exec.Command(
		"docker", "pull", helperImage,
	))

	Run(t, exec.Command(
		"docker", "run",
		"--volume", volumeName+":/target", // create a new empty docker volume
		"--name", containerName,
		helperImage,
		"true", // TODO: make this OS-agnostic
	))
	defer Run(t, exec.Command("docker", "rm", containerName))

	fis, err := ioutil.ReadDir(srcPath)
	AssertNil(t, err)
	for _, fi := range fis {
		Run(t, exec.Command(
			"docker", "cp",
			filepath.Join(srcPath, fi.Name()),
			containerName+":/target",
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
