package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
)

func TestKnative(t *testing.T) {
	spec.Run(t, "Knative", testKnative, spec.Report(report.Terminal{}))
}

func testKnative(t *testing.T, when spec.G, it spec.S) {
	var workspace string

	it.Before(func() {
		var err error
		workspace, err = ioutil.TempDir("", "knative-test-workspace")
		if err != nil {
			t.Fatalf("Error: creating temp workspace dir: %s\n", err)
		}
		file1, err := os.Create(filepath.Join(workspace, "file1.txt"))
		if err != nil {
			t.Fatalf("Error: creating test file: %s\n", err)
		}
		defer file1.Close()
		file1.Write([]byte("file1 contents"))
		if err := os.Mkdir(filepath.Join(workspace, "app"), 0755); err != nil {
			t.Fatalf("Error: creating app dir in workspace: %s\n", err)
		}
		file2, err := os.Create(filepath.Join(workspace, "app", "file2.txt"))
		if err != nil {
			t.Fatalf("Error: creating test file: %s\n", err)
		}
		defer file2.Close()
		file2.Write([]byte("file2 contents"))
	})

	it.After(func() {
		if err := os.RemoveAll(workspace); err != nil {
			t.Fatalf("Error: removing temp workspace dir: %s\n", err)
		}
	})

	it("moves the contents of /workspace to /workspace/app and chowns /builder/home", func() {
		if err := lifecycle.SetupKnativeLaunchDir(workspace); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		file1, err := os.Open(filepath.Join(workspace, "app", "file1.txt"))
		if err != nil {
			t.Fatalf("Error: opening <workspace>/app/file1.txt: %s\n", err)
		}
		contents, err := ioutil.ReadAll(file1)
		if err != nil {
			t.Fatalf("Error: reading <workspace>/app/file1.txt: %s\n", err)
		}
		if string(contents) != "file1 contents" {
			t.Fatalf(`Error: contents of  <workspace>/app/file1.txt: got %s, expected "file1 contents"`, contents)
		}

		file2, err := os.Open(filepath.Join(workspace, "app", "app", "file2.txt"))
		if err != nil {
			t.Fatalf("Error: opening <workspace>/app/app/file2.txt: %s\n", err)
		}
		contents, err = ioutil.ReadAll(file2)
		if err != nil {
			t.Fatalf("Error: reading <workspace>/app/app/file2.txt: %s\n", err)
		}
		if string(contents) != "file2 contents" {
			t.Fatalf(`Error: contents of  <workspace>/app/app/file2.txt: got %s, expected "file2 contents"`, contents)
		}
	})
}
