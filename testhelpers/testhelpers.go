package testhelpers

import (
	"archive/tar"
	"bytes"
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
	"testing"
	"time"

	"github.com/apex/log/handlers/memory"
	"github.com/google/go-cmp/cmp"
)

func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26)) // #nosec: G404
	}
	return string(b)
}

func SkipIf(t *testing.T, expression bool, reason string) {
	t.Helper()
	if expression {
		t.Skip(reason)
	}
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

func AssertStringDoesNotContain(t *testing.T, str string, expected string) {
	t.Helper()
	if strings.Contains(str, expected) {
		t.Fatalf("Expected %s not to contain: %s\n", str, expected)
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

func AssertNotNil(t *testing.T, actual interface{}) {
	t.Helper()
	if isNil(actual) {
		t.Fatal("Expected not nil")
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
	t.Helper()
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		t.Errorf("Expected %q to exist", path)
	} else if err != nil {
		t.Fatalf("Error stating %q: %v", path, err)
	}
}

func AssertPathDoesNotExist(t *testing.T, path string) {
	t.Helper()
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
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		t.Fatalf("failed to copy file to hasher: %s", err)
	}

	return hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
}

func RecursiveCopy(t *testing.T, src, dst string) {
	t.Helper()
	fis, err := ioutil.ReadDir(src)
	AssertNil(t, err)
	for _, fi := range fis {
		if fi.Mode().IsRegular() {
			CopyFile(t, filepath.Join(src, fi.Name()), filepath.Join(dst, fi.Name()))
		}
		if fi.IsDir() {
			err = os.Mkdir(filepath.Join(dst, fi.Name()), fi.Mode())
			AssertNil(t, err)
			RecursiveCopy(t, filepath.Join(src, fi.Name()), filepath.Join(dst, fi.Name()))
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filepath.Join(src, fi.Name()))
			AssertNil(t, err)
			if filepath.IsAbs(target) {
				t.Fatalf("symlinks cannot be absolute")
			}
			AssertNil(t, os.Symlink(target, filepath.Join(dst, fi.Name())))
		}
	}
	modifiedtime := time.Time{}
	err = os.Chtimes(dst, modifiedtime, modifiedtime)
	AssertNil(t, err)
	err = os.Chmod(dst, 0775)
	AssertNil(t, err)
}

func CopyFile(t *testing.T, srcFileName, destFileName string) {
	srcFile, err := os.Open(srcFileName)
	AssertNil(t, err)
	defer srcFile.Close()
	dstFile, err := os.Create(destFileName)
	AssertNil(t, err)
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	AssertNil(t, err)
	modifiedtime := time.Time{}
	err = os.Chtimes(destFileName, modifiedtime, modifiedtime)
	AssertNil(t, err)
	err = os.Chmod(destFileName, 0664)
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

func RandomLayer(t *testing.T, tmpDir string) (path string, diffID string, contents []byte) {
	t.Helper()

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

	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	return path, "sha256:" + sha, contentsBuf.Bytes()
}

func MustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Error reading %q: %v", path, err)
	}
	return data
}

func Mkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func Mkfile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := ioutil.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func CleanEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func Rdfile(t *testing.T, path string) string {
	t.Helper()
	out, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Error: %s\n", err)
	}
	return CleanEndings(string(out))
}

func AllLogs(logHandler *memory.Handler) string {
	var out string
	for _, le := range logHandler.Entries {
		out = out + le.Message + "\n"
	}
	return CleanEndings(out)
}
