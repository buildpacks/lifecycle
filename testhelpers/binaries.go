package testhelpers

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func MakeAndCopyLauncher(t *testing.T, goos, destDir string) {
	buildDir, err := filepath.Abs(filepath.Join("..", "out"))
	AssertNil(t, err)

	cmd := exec.Command("make", fmt.Sprintf("build-%s-launcher", goos)) // #nosec G204

	wd, err := os.Getwd()
	AssertNil(t, err)
	cmd.Dir = filepath.Join(wd, "..")

	cmd.Env = append(
		os.Environ(),
		"PWD="+cmd.Dir,
		"BUILD_DIR="+buildDir,
	)

	t.Log("Building binaries: ", cmd.Args)
	Run(t, cmd)

	copyLauncher(t, filepath.Join(buildDir, goos, "lifecycle"), destDir)
}

func MakeAndCopyLifecycle(t *testing.T, goos, destDir string, envs ...string) {
	buildDir, err := filepath.Abs(filepath.Join("..", "out"))
	AssertNil(t, err)

	cmd := exec.Command("make", "build-"+goos) // #nosec G204

	wd, err := os.Getwd()
	AssertNil(t, err)
	cmd.Dir = filepath.Join(wd, "..")

	envs = append(
		envs,
		"PWD="+cmd.Dir,
		"BUILD_DIR="+buildDir,
	)
	cmd.Env = append(os.Environ(), envs...)

	t.Log("Building binaries: ", cmd.Args)
	Run(t, cmd)

	copyLifecycle(t, filepath.Join(buildDir, goos, "lifecycle"), destDir)
}

func copyLauncher(t *testing.T, src, dst string) {
	AssertNil(t, os.RemoveAll(dst)) // Clear any existing binaries
	AssertNil(t, os.MkdirAll(dst, 0755))

	binaryName := "launcher"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	// Copy launcher
	CopyFile(t, filepath.Join(src, binaryName), filepath.Join(dst, binaryName))

	// Ensure correct permissions
	AssertNil(t, os.Chmod(filepath.Join(dst, binaryName), 0755))
}

func copyLifecycle(t *testing.T, src, dst string) {
	AssertNil(t, os.RemoveAll(dst)) // Clear any existing binaries
	AssertNil(t, os.MkdirAll(dst, 0755))

	// Copy lifecycle binaries
	RecursiveCopy(t, src, dst)

	// Ensure correct permissions
	fis, err := ioutil.ReadDir(dst)
	AssertNil(t, err)

	for _, fi := range fis {
		AssertNil(t, os.Chmod(filepath.Join(dst, fi.Name()), 0755))
	}
}
