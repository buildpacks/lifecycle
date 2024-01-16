package testhelpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func MakeAndCopyLifecycle(t *testing.T, goos, goarch, destDir string, envs ...string) {
	outParentDir, err := filepath.Abs(filepath.Join("..", "out"))
	AssertNil(t, err)

	makeCmd := "make"
	if runtime.GOOS == "freebsd" {
		makeCmd = "gmake"
	}

	cmd := exec.Command(makeCmd, fmt.Sprintf("build-%s-%s", goos, goarch)) // #nosec G204

	wd, err := os.Getwd()
	AssertNil(t, err)
	cmd.Dir = filepath.Join(wd, "..")

	envs = append(
		envs,
		"PWD="+cmd.Dir,
		"BUILD_DIR="+outParentDir,
	)
	cmd.Env = append(os.Environ(), envs...)

	t.Log("Building binaries:", cmd.Args)
	output := Run(t, cmd)
	t.Log(output)

	outDir := filepath.Join(outParentDir, fmt.Sprintf("%s-%s", goos, goarch), "lifecycle")
	copyLifecycle(t, outDir, destDir)
}

func copyLifecycle(t *testing.T, src, dst string) {
	AssertNil(t, os.RemoveAll(dst)) // Clear any existing binaries
	AssertNil(t, os.MkdirAll(dst, 0755))

	// Copy lifecycle binaries
	RecursiveCopy(t, src, dst)

	// Ensure correct permissions
	fis, err := os.ReadDir(dst)
	AssertNil(t, err)

	for _, fi := range fis {
		AssertNil(t, os.Chmod(filepath.Join(dst, fi.Name()), 0755))
	}
}
