package testhelpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/moby/api/types/image"
	dockercli "github.com/moby/moby/client"
	"github.com/pkg/errors"
)

var (
	dockerCliOnce sync.Once
	dockerCliVal  dockercli.APIClient
)

// DockerCli returns a new docker client
func DockerCli(t *testing.T) dockercli.APIClient {
	dockerCliOnce.Do(func() {
		var dockerCliErr error
		dockerCliVal, dockerCliErr = dockercli.New(dockercli.FromEnv)
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
	Run(t, exec.Command("docker", "rmi", name, "--force")) // #nosec G204
}

// DockerImageRemoveSafe removes a Docker image without failing the test if the image doesn't exist
// This is useful for cleanup in defer statements where the image may have already been removed
func DockerImageRemoveSafe(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command("docker", "image", "rm", name, "--force") // #nosec G204
	output, exitCode, err := RunE(cmd)

	// Only log if there was an error other than "no such image"
	if err != nil && !strings.Contains(output, "No such image") && !strings.Contains(output, "unrecognized image") {
		t.Logf("Warning: Failed to remove image %s (exit code %d): %s", name, exitCode, output)
	}
}

func DockerRun(t *testing.T, image string, ops ...DockerCmdOp) string {
	t.Helper()
	args := formatArgs([]string{image}, ops...)
	return Run(t, exec.Command("docker", append([]string{"run", "--rm"}, args...)...)) // #nosec G204
}

// DockerRunWithError allows to run docker command that might fail, reporting the error back to the caller
func DockerRunWithError(t *testing.T, image string, ops ...DockerCmdOp) (string, int, error) {
	t.Helper()
	args := formatArgs([]string{image}, ops...)
	return RunE(exec.Command("docker", append([]string{"run", "--rm"}, args...)...)) // #nosec G204
}

func DockerRunWithCombinedOutput(t *testing.T, image string, ops ...DockerCmdOp) string {
	t.Helper()
	args := formatArgs([]string{image}, ops...)
	return RunWithCombinedOutput(t, exec.Command("docker", append([]string{"run", "--rm"}, args...)...)) // #nosec G204
}

// DockerRunAndCopy runs a container and once stopped, outputCtrPath is copied to outputDir
func DockerRunAndCopy(t *testing.T, containerName, outputDir, outputCtrPath, image string, ops ...DockerCmdOp) string {
	ops = append(ops, WithFlags("--name", containerName))
	args := formatArgs([]string{image}, ops...)

	output := Run(t, exec.Command("docker", append([]string{"run"}, args...)...))    // #nosec G204
	Run(t, exec.Command("docker", "cp", containerName+":"+outputCtrPath, outputDir)) // #nosec G204
	return output
}

// DockerSeedRunAndCopy copies srcDir to container's srcCtrPath before container is started. Once stopped, outputCtrPath is copied to outputDir
// On WCOW, only works when seeding to container directory (not a mounted volume)
func DockerSeedRunAndCopy(t *testing.T, containerName, srcDir, srcCtrPath, outputDir, outputCtrPath, image string, ops ...DockerCmdOp) string {
	ops = append(ops, WithFlags("--name", containerName))
	args := formatArgs([]string{image}, ops...)

	output := Run(t, exec.Command("docker", append([]string{"create"}, args...)...))           // #nosec G204
	output += Run(t, exec.Command("docker", "cp", srcDir, containerName+":"+srcCtrPath))       // #nosec G204
	output += Run(t, exec.Command("docker", "start", "--attach", containerName))               // #nosec G204
	output += Run(t, exec.Command("docker", "cp", containerName+":"+outputCtrPath, outputDir)) // #nosec G204

	return output
}

func DockerCopyOut(t *testing.T, containerName, srcCtrPath, outputDir string) string {
	return Run(t, exec.Command("docker", "cp", containerName+":"+srcCtrPath, outputDir)) // #nosec G204
}

func DockerContainerExists(t *testing.T, containerName string) bool {
	output := Run(t, exec.Command("docker", "ps", "-a"))
	return strings.Contains(output, containerName)
}

func DockerVolumeRemove(t *testing.T, volume string) {
	Run(t, exec.Command("docker", "volume", "rm", volume)) // #nosec G204
}

func DockerVolumeExists(t *testing.T, volumeName string) bool {
	if volumeName == "" {
		return false
	}
	output := Run(t, exec.Command("docker", "volume", "ls"))
	return strings.Contains(output, volumeName)
}

// FIXME: re-work this function to exec the docker cli, or convert other docker helpers to using the client library.
func PushImage(dockerCli dockercli.APIClient, ref string, auth string) error {
	rc, err := dockerCli.ImagePush(context.Background(), ref, dockercli.ImagePushOptions{RegistryAuth: auth})
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

// SeedDockerVolume only works with Linux daemons
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
	defer Run(t, exec.Command("docker", "rm", containerName)) // #nosec G204

	fis, err := os.ReadDir(srcPath)
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
	body, err := io.ReadAll(responseBody)
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

// DockerPullWithRetry pulls a Docker image with retry logic
// This is useful when pulling from local registries that may have timing issues
func DockerPullWithRetry(t *testing.T, imageName string) error {
	t.Helper()

	maxRetries := 5
	backoffDelays := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second, 2 * time.Second}

	var lastErr error
	for attempt := range maxRetries {
		cmd := exec.Command("docker", "pull", imageName) // #nosec G204
		output, exitCode, err := RunE(cmd)

		if err == nil {
			if attempt > 0 {
				t.Logf("Successfully pulled image %q after %d retries", imageName, attempt)
			}
			t.Logf("Pull output for %q: %s", imageName, output)
			// Verify the image is available via Docker API before returning
			// This ensures the image is fully indexed and available for inspection
			inspect, err := DockerCli(t).ImageInspect(context.TODO(), imageName)
			if err == nil {
				t.Logf("Image %q successfully pulled and verified (ID: %s)", imageName, inspect.ID)
				return nil
			}
			t.Logf("Image %q pulled but not yet available via API (attempt %d/%d), will retry: %v", imageName, attempt+1, maxRetries, err)
			lastErr = err
		} else {
			lastErr = err
			// Log diagnostic information
			t.Logf("Attempt %d/%d: Failed to pull image %q (exit code %d): %s", attempt+1, maxRetries, imageName, exitCode, output)
		}

		// Wait before retrying (unless this is the last attempt)
		if attempt < maxRetries-1 {
			t.Logf("Retrying after %v", backoffDelays[attempt])
			time.Sleep(backoffDelays[attempt])
		}
	}

	// All retries exhausted - try one more time to get detailed output
	t.Logf("All retries exhausted for pulling image %q", imageName)
	cmd := exec.Command("docker", "pull", imageName) // #nosec G204
	output, exitCode, _ := RunE(cmd)
	t.Logf("Final pull attempt output (exit code %d): %s", exitCode, output)

	return fmt.Errorf("failed to pull image %q after %d retries, last error: %w", imageName, maxRetries, lastErr)
}

// ImageInspectWithRetry attempts to inspect a Docker image with retry logic and detailed diagnostics
// This is useful for handling race conditions where images may not be immediately available
func ImageInspectWithRetry(t *testing.T, dockerCli dockercli.APIClient, imageName string) (image.InspectResponse, error) {
	t.Helper()

	maxRetries := 5
	backoffDelays := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second, 2 * time.Second}

	var lastErr error
	for attempt := range maxRetries {
		inspect, err := dockerCli.ImageInspect(context.TODO(), imageName)
		if err == nil {
			if attempt > 0 {
				t.Logf("Successfully inspected image %q after %d retries", imageName, attempt)
			}
			return inspect.InspectResponse, nil
		}

		lastErr = err

		// Log diagnostic information on error
		if strings.Contains(err.Error(), "No such image") {
			t.Logf("Attempt %d/%d: Image %q not found, will retry after %v", attempt+1, maxRetries, imageName, backoffDelays[attempt])

			// List available images for debugging
			if attempt == 0 {
				list, listErr := dockerCli.ImageList(context.TODO(), dockercli.ImageListOptions{})
				if listErr == nil {
					t.Logf("Available images in daemon:")
					for _, img := range list.Items {
						t.Logf("  - RepoTags: %v, RepoDigests: %v, ID: %s", img.RepoTags, img.RepoDigests, img.ID)
					}
				} else {
					t.Logf("Failed to list images: %v", listErr)
				}
			}

			// Wait before retrying (unless this is the last attempt)
			if attempt < maxRetries-1 {
				time.Sleep(backoffDelays[attempt])
			}
		} else {
			// For non-"No such image" errors, don't retry
			t.Logf("ImageInspect failed with non-retryable error: %v", err)
			return image.InspectResponse{}, err
		}
	}

	// All retries exhausted
	t.Logf("Failed to inspect image %q after %d attempts. Last error: %v", imageName, maxRetries, lastErr)
	return image.InspectResponse{}, fmt.Errorf("failed to inspect image after %d retries: %w", maxRetries, lastErr)
}
