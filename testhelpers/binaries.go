package testhelpers

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func BuildLinuxLauncher(t *testing.T, buildDir string) {
	cmd := exec.Command("make", "build-linux-launcher")
	wd, err := os.Getwd()
	AssertNil(t, err)
	cmd.Dir = filepath.Join(wd, "..")
	cmd.Env = append(
		os.Environ(),
		"PWD="+cmd.Dir,
		"BUILD_DIR="+buildDir,
		"LIFECYCLE_VERSION=some-version",
		"SCM_COMMIT=asdf123",
	)

	t.Log("Building binaries: ", cmd.Args)
	Run(t, cmd)
}

func BuildLifecycle(t *testing.T, buildDir string, goos string) {
	cmd := exec.Command("make", "build-"+goos)
	wd, err := os.Getwd()
	AssertNil(t, err)
	cmd.Dir = filepath.Join(wd, "..")
	cmd.Env = append(
		os.Environ(),
		"GOOS="+goos,
		"PWD="+cmd.Dir,
		"BUILD_DIR="+buildDir,
		"PLATFORM_API=0.9",
		"LIFECYCLE_VERSION=some-version",
		"SCM_COMMIT=asdf123",
	)

	t.Log("Building binaries: ", cmd.Args)
	Run(t, cmd)
}

func CopyLauncher(t *testing.T, src, dst string) {
	AssertNil(t, os.RemoveAll(dst)) // Clear any existing binaries
	AssertNil(t, os.MkdirAll(dst, 0755))

	// Copy launcher
	CopyFile(t, filepath.Join(src, "launcher"), filepath.Join(dst, "launcher"))

	// Ensure correct permissions
	AssertNil(t, os.Chmod(filepath.Join(dst, "launcher"), 0755))
}

func CopyLifecycle(t *testing.T, src, dst string) {
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

	// Copy lifecycle symlinks
	fis, err = ioutil.ReadDir(src)
	AssertNil(t, err)

	for _, fi := range fis {
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			currentTarget, err := os.Readlink(filepath.Join(src, fi.Name()))
			AssertNil(t, err)

			newTarget := filepath.Base(currentTarget) // assume the target file is in the destination directory
			newSource := filepath.Join(dst, fi.Name())
			os.RemoveAll(newSource)

			AssertNil(t, os.Symlink(newTarget, newSource))
		}
	}
}
