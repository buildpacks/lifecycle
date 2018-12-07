package testhelpers

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	dockerClient "github.com/docker/docker/client"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buildpack/lifecycle/fs"
	"github.com/dgodd/dockerdial"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-cmp/cmp"
)

func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

// Assert deep equality (and provide useful difference as a test failure)
func AssertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

func AssertContains(t *testing.T, slice []string, expected string) {
	t.Helper()
	for _, actual := range slice {
		if diff := cmp.Diff(actual, expected); diff == "" {
			return
		}
	}
	t.Fatalf("Expected %+v to contain: %s", slice, expected)

}

func AssertMatch(t *testing.T, actual string, expected *regexp.Regexp) {
	t.Helper()
	if !expected.Match([]byte(actual)) {
		t.Fatal(cmp.Diff(actual, expected))
	}
}

func AssertError(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatalf("Expected an error but got nil")
	}
	if !strings.Contains(actual.Error(), expected) {
		t.Fatalf(`Expected error to contain "%s", got "%s"`, expected, actual.Error())
	}
}

func AssertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

var dockerCliVal *dockerClient.Client
var dockerCliOnce sync.Once

func DockerCli(t *testing.T) *dockerClient.Client {
	dockerCliOnce.Do(func() {
		var dockerCliErr error
		dockerCliVal, dockerCliErr = dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithVersion("1.38"))
		AssertNil(t, dockerCliErr)
	})
	return dockerCliVal
}

func proxyDockerHostPort(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	go func() {
		// TODO exit somehow.
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println(err)
				continue
			}
			go func(conn net.Conn) {
				defer conn.Close()
				c, err := dockerdial.Dial("tcp", "localhost:"+port)
				if err != nil {
					log.Println(err)
					return
				}
				defer c.Close()

				go io.Copy(c, conn)
				io.Copy(conn, c)
			}(conn)
		}
	}()
	return nil
}

var runRegistryName, runRegistryPort string
var runRegistryOnce sync.Once

func RunRegistry(t *testing.T, seedRegistry bool) (localPort string) {
	t.Log("run registry")
	t.Helper()
	runRegistryOnce.Do(func() {
		runRegistryName = "test-registry-" + RandString(10)

		AssertNil(t, PullImage(DockerCli(t), "registry:2"))
		ctx := context.Background()
		ctr, err := DockerCli(t).ContainerCreate(ctx, &container.Config{
			Image: "registry:2",
		}, &container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				"5000/tcp": []nat.PortBinding{{}},
			},
		}, nil, runRegistryName)
		AssertNil(t, err)
		defer DockerCli(t).ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})
		err = DockerCli(t).ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
		AssertNil(t, err)

		inspect, err := DockerCli(t).ContainerInspect(context.TODO(), ctr.ID)
		AssertNil(t, err)
		runRegistryPort = inspect.NetworkSettings.Ports["5000/tcp"][0].HostPort

		if os.Getenv("DOCKER_HOST") != "" {
			err := proxyDockerHostPort(runRegistryPort)
			AssertNil(t, err)
		}

		Eventually(t, func() bool {
			txt, err := HttpGetE(fmt.Sprintf("http://localhost:%s/v2/", runRegistryPort))
			return err == nil && txt != ""
		}, 100*time.Millisecond, 10*time.Second)

		if seedRegistry {
			t.Log("seed registry")
			for _, f := range []func(*testing.T, string) string{DefaultBuildImage, DefaultRunImage, DefaultBuilderImage} {
				AssertNil(t, pushImage(DockerCli(t), f(t, runRegistryPort)))
			}
		}
	})
	return runRegistryPort
}

func Eventually(t *testing.T, test func() bool, every time.Duration, timeout time.Duration) {
	t.Helper()

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if test() {
				return
			}
		case <-timer.C:
			t.Fatalf("timeout on eventually: %v", timeout)
		}
	}
}

func StopRegistry(t *testing.T) {
	t.Log("stop registry")
	t.Helper()
	if runRegistryName != "" {
		DockerCli(t).ContainerKill(context.Background(), runRegistryName, "SIGKILL")
		DockerCli(t).ContainerRemove(context.TODO(), runRegistryName, dockertypes.ContainerRemoveOptions{Force: true})
	}
}

var getBuildImageOnce sync.Once

func DefaultBuildImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getBuildImageOnce.Do(func() {
		if tag == "latest" {
			AssertNil(t, PullImage(DockerCli(t), fmt.Sprintf("packs/build:%s", tag)))
		}
		AssertNil(t, DockerCli(t).ImageTag(
			context.Background(),
			fmt.Sprintf("packs/build:%s", tag),
			fmt.Sprintf("localhost:%s/packs/build:%s", registryPort, tag),
		))
	})
	return fmt.Sprintf("localhost:%s/packs/build:%s", registryPort, tag)
}

var getRunImageOnce sync.Once

func DefaultRunImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getRunImageOnce.Do(func() {
		if tag == "latest" {
			AssertNil(t, PullImage(DockerCli(t), fmt.Sprintf("packs/run:%s", tag)))
		}
		AssertNil(t, DockerCli(t).ImageTag(
			context.Background(),
			fmt.Sprintf("packs/run:%s", tag),
			fmt.Sprintf("localhost:%s/packs/run:%s", registryPort, tag),
		))
	})
	return fmt.Sprintf("localhost:%s/packs/run:%s", registryPort, tag)
}

var getBuilderImageOnce sync.Once

func DefaultBuilderImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getBuilderImageOnce.Do(func() {
		if tag == "latest" {
			AssertNil(t, PullImage(DockerCli(t), fmt.Sprintf("packs/samples:%s", tag)))
		}
		AssertNil(t, DockerCli(t).ImageTag(
			context.Background(),
			fmt.Sprintf("packs/samples:%s", tag),
			fmt.Sprintf("localhost:%s/packs/samples:%s", registryPort, tag),
		))
	})
	return fmt.Sprintf("localhost:%s/packs/samples:%s", registryPort, tag)
}

func CreateImageOnLocal(t *testing.T, dockerCli *dockerClient.Client, repoName, dockerFile string) {
	ctx := context.Background()

	buildContext, err := (&fs.FS{}).CreateSingleFileTar("Dockerfile", dockerFile)
	AssertNil(t, err)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:           []string{repoName},
		SuppressOutput: true,
		Remove:         true,
		ForceRemove:    true,
	})
	AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
}

func CreateImageOnRemote(t *testing.T, dockerCli *dockerClient.Client, repoName, dockerFile string) string {
	t.Helper()
	defer DockerRmi(dockerCli, repoName)

	CreateImageOnLocal(t, dockerCli, repoName, dockerFile)

	var topLayer string
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), repoName)
	AssertNil(t, err)
	if len(inspect.RootFS.Layers) > 0 {
		topLayer = inspect.RootFS.Layers[len(inspect.RootFS.Layers)-1]
	} else {
		topLayer = "N/A"
	}

	AssertNil(t, pushImage(dockerCli, repoName))

	return topLayer
}

func PullImage(dockerCli *dockerClient.Client, ref string) error {
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

func DockerRmi(dockerCli *dockerClient.Client, repoNames ...string) error {
	var err error
	ctx := context.Background()
	for _, name := range repoNames {
		_, e := dockerCli.ImageRemove(
			ctx,
			name,
			dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true},
		)
		if e != nil && err == nil {
			err = e
		}
	}
	return err
}

func CopySingleFileFromContainer(dockerCli *dockerClient.Client, ctrID, path string) (string, error) {
	r, _, err := dockerCli.CopyFromContainer(context.Background(), ctrID, path)
	if err != nil {
		return "", err
	}
	defer r.Close()
	tr := tar.NewReader(r)
	hdr, err := tr.Next()
	if hdr.Name != path && hdr.Name != filepath.Base(path) {
		return "", fmt.Errorf("filenames did not match: %s and %s (%s)", hdr.Name, path, filepath.Base(path))
	}
	b, err := ioutil.ReadAll(tr)
	return string(b), err
}

func CopySingleFileFromImage(dockerCli *dockerClient.Client, repoName, path string) (string, error) {
	ctr, err := dockerCli.ContainerCreate(context.Background(),
		&container.Config{
			Image: repoName,
		}, &container.HostConfig{
			AutoRemove: true,
		}, nil, "",
	)
	if err != nil {
		return "", err
	}
	defer dockerCli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{})
	return CopySingleFileFromContainer(dockerCli, ctr.ID, path)
}

func pushImage(dockerCli *dockerClient.Client, ref string) error {
	rc, err := dockerCli.ImagePush(context.Background(), ref, dockertypes.ImagePushOptions{RegistryAuth: "{}"})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func packTag() string {
	tag := os.Getenv("PACK_TAG")
	if tag == "" {
		return "latest"
	}
	return tag
}

func HttpGetE(url string) (string, error) {
	var client *http.Client
	if os.Getenv("DOCKER_HOST") == "" {
		client = http.DefaultClient
	} else {
		tr := &http.Transport{Dial: dockerdial.Dial}
		client = &http.Client{Transport: tr}
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP Status was bad: %s => %d", url, resp.StatusCode)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ImageID(t *testing.T, repoName string) string {
	t.Helper()
	inspect, _, err := DockerCli(t).ImageInspectWithRaw(context.Background(), repoName)
	AssertNil(t, err)
	return inspect.ID
}

func Run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	txt, err := RunE(cmd)
	AssertNil(t, err)
	return txt
}

func RunE(cmd *exec.Cmd) (string, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Failed to execute command: %v, %s, %s, %s", cmd.Args, err, stderr.String(), output)
	}

	return string(output), nil
}

func ComputeSHA256(t *testing.T, path string) string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file: %s", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		t.Fatalf("failed to copy file to hasher: %s", err)
	}

	return hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
}
