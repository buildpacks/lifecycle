package testhelpers

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-cmp/cmp"

	"github.com/buildpacks/lifecycle/archive"
)

func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

func AssertMatch(t *testing.T, actual string, expected string) {
	t.Helper()
	if !regexp.MustCompile(expected).MatchString(actual) {
		t.Fatalf("Expected: '%s' to match regex '%s'", actual, expected)
	}
}

// Assert the simplistic pointer (or literal value) equality
func AssertSameInstance(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if actual != expected {
		t.Fatalf("Expected %s and %s to be pointers to the variable", actual, expected)
	}
}

// Assert deep equality (and provide useful difference as a test failure)
func AssertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

func AssertContains(t *testing.T, slice []string, elements ...string) {
	t.Helper()

outer:
	for _, el := range elements {
		for _, actual := range slice {
			if diff := cmp.Diff(actual, el); diff == "" {
				continue outer
			}
		}

		t.Fatalf("Expected %+v to contain: %s", slice, el)
	}
}

func AssertStringContains(t *testing.T, str string, expected string) {
	t.Helper()
	if !strings.Contains(str, expected) {
		t.Fatalf("Expected %s to contain: %s\nDiff:\n%s", str, expected, cmp.Diff(str, expected))
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
	if !isNil(actual) {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func AssertJSONEq(t *testing.T, expected, actual string) {
	t.Helper()

	var expectedJSONAsInterface, actualJSONAsInterface interface{}

	if err := json.Unmarshal([]byte(expected), &expectedJSONAsInterface); err != nil {
		t.Fatalf("Expected value ('%s') is not valid json.\nJSON parsing error: '%s'", expected, err.Error())
	}

	if err := json.Unmarshal([]byte(actual), &actualJSONAsInterface); err != nil {
		t.Fatalf("Input ('%s') needs to be valid json.\nJSON parsing error: '%s'", actual, err.Error())
	}

	AssertEq(t, expectedJSONAsInterface, actualJSONAsInterface)
}

func AssertPathExists(t *testing.T, path string) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		t.Errorf("Expected %q to exist", path)
	} else if err != nil {
		t.Fatalf("Error stating %q: %v", path, err)
	}
}

func AssertPathDoesNotExist(t *testing.T, path string) {
	_, err := os.Stat(path)
	if err == nil {
		t.Errorf("Expected %q to not exist", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Error stating %q: %v", path, err)
	}
}

func isNil(value interface{}) bool {
	return value == nil || (reflect.TypeOf(value).Kind() == reflect.Ptr && reflect.ValueOf(value).IsNil())
}

var dockerCliVal *dockercli.Client
var dockerCliOnce sync.Once

func DockerCli(t *testing.T) *dockercli.Client {
	dockerCliOnce.Do(func() {
		var dockerCliErr error
		dockerCliVal, dockerCliErr = dockercli.NewClientWithOpts(dockercli.FromEnv, dockercli.WithVersion("1.38"))
		AssertNil(t, dockerCliErr)
	})
	return dockerCliVal
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

func HTTPGetE(url string) (string, error) {
	resp, err := http.DefaultClient.Get(url)
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
	txt, _, err := RunE(cmd)
	AssertNil(t, err)
	return txt
}

func RunE(cmd *exec.Cmd) (output string, exitCode int, err error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()
	if err != nil {
		formattedErr := fmt.Errorf("failed to execute command: %v, %s, %s, %s", cmd.Args, err, stderr.String(), stdout)
		if exitError, ok := err.(*exec.ExitError); ok {
			return "", exitError.ExitCode(), formattedErr
		}

		return "", -1, formattedErr
	}

	return string(stdout), 0, nil
}

func ComputeSHA256ForFile(t *testing.T, path string) string {
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

func ComputeSHA256ForPath(t *testing.T, path string, uid int, guid int) string {
	hasher := sha256.New()
	err := archive.WriteTarArchive(hasher, archive.DefaultTarWriterFactory(), path, uid, guid)
	AssertNil(t, err)
	layer5sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
	return layer5sha
}

func ComputeSHA256ForFiles(t *testing.T, path string, uid int, guid int, files ...string) string {
	sha, _, err := archive.WriteFilesToTar(path, uid, guid, archive.DefaultTarWriterFactory(), files...)
	AssertNil(t, err)
	return sha[len("sha256:"):]
}

func RecursiveCopy(t *testing.T, src, dst string) {
	t.Helper()
	fis, err := ioutil.ReadDir(src)
	AssertNil(t, err)
	for _, fi := range fis {
		if fi.Mode().IsRegular() {
			srcFile, err := os.Open(filepath.Join(src, fi.Name()))
			AssertNil(t, err)
			dstFile, err := os.Create(filepath.Join(dst, fi.Name()))
			AssertNil(t, err)
			_, err = io.Copy(dstFile, srcFile)
			AssertNil(t, err)
			modifiedtime := time.Time{}
			AssertNil(t, srcFile.Close())
			AssertNil(t, dstFile.Close())
			err = os.Chtimes(filepath.Join(dst, fi.Name()), modifiedtime, modifiedtime)
			AssertNil(t, err)
			err = os.Chmod(filepath.Join(dst, fi.Name()), 0664)
			AssertNil(t, err)
		}
		if fi.IsDir() {
			err = os.Mkdir(filepath.Join(dst, fi.Name()), fi.Mode())
			AssertNil(t, err)
			RecursiveCopy(t, filepath.Join(src, fi.Name()), filepath.Join(dst, fi.Name()))
		}
	}
	modifiedtime := time.Time{}
	err = os.Chtimes(dst, modifiedtime, modifiedtime)
	AssertNil(t, err)
	err = os.Chmod(dst, 0775)
	AssertNil(t, err)
}

func CreateSingleFileTar(path, txt string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(txt)), Mode: 0644}); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(txt)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return bytes.NewReader(buf.Bytes()), nil
}

func RandomLayer(t *testing.T, tmpDir string) (path string, sha string, contents []byte) {
	r, err := CreateSingleFileTar("/some-file", RandString(10))
	AssertNil(t, err)

	path = filepath.Join(tmpDir, RandString(10)+".tar")
	fh, err := os.Create(path)
	AssertNil(t, err)
	defer fh.Close()

	hasher := sha256.New()
	var contentsBuf bytes.Buffer
	mw := io.MultiWriter(hasher, fh, &contentsBuf)

	_, err = io.Copy(mw, r)
	AssertNil(t, err)

	sha = hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	return path, "sha256:" + sha, contentsBuf.Bytes()
}

func MustReadFile(t *testing.T, path string) []byte {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Error reading %q: %v", path, err)
	}
	return data
}
