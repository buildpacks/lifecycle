package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSystem(t *testing.T) {
	spec.Run(t, "System", testSystem, spec.Report(report.Terminal{}))
}

func testSystem(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir string
		logger *log.DefaultLogger
	)

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle.test")
		h.AssertNil(t, err)
		logger = log.NewDefaultLogger(os.Stdout)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#ReadSystem", func() {
		when("system.toml exists with pre and post buildpacks", func() {
			it("returns system buildpacks", func() {
				systemTOMLContents := `
[system]
  [system.pre]
    [[system.pre.buildpacks]]
      id = "pre-buildpack-1"
      version = "1.0.0"

    [[system.pre.buildpacks]]
      id = "pre-buildpack-2"
      version = "2.0.0"

  [system.post]
    [[system.post.buildpacks]]
      id = "post-buildpack-1"
      version = "1.5.0"
`
				h.Mkfile(t, systemTOMLContents, filepath.Join(tmpDir, "system.toml"))

				handler := files.NewHandler()
				system, err := handler.ReadSystem(filepath.Join(tmpDir, "system.toml"), logger)

				h.AssertNil(t, err)
				h.AssertEq(t, len(system.Pre.Buildpacks), 2)
				h.AssertEq(t, system.Pre.Buildpacks[0].ID, "pre-buildpack-1")
				h.AssertEq(t, system.Pre.Buildpacks[0].Version, "1.0.0")
				h.AssertEq(t, system.Pre.Buildpacks[1].ID, "pre-buildpack-2")
				h.AssertEq(t, system.Pre.Buildpacks[1].Version, "2.0.0")
				h.AssertEq(t, len(system.Post.Buildpacks), 1)
				h.AssertEq(t, system.Post.Buildpacks[0].ID, "post-buildpack-1")
				h.AssertEq(t, system.Post.Buildpacks[0].Version, "1.5.0")
			})
		})

		when("system.toml exists with only pre buildpacks", func() {
			it("returns only pre buildpacks", func() {
				systemTOMLContents := `
[system]
  [system.pre]
    [[system.pre.buildpacks]]
      id = "pre-only"
      version = "0.1.0"
`
				h.Mkfile(t, systemTOMLContents, filepath.Join(tmpDir, "system.toml"))

				handler := files.NewHandler()
				system, err := handler.ReadSystem(filepath.Join(tmpDir, "system.toml"), logger)

				h.AssertNil(t, err)
				h.AssertEq(t, len(system.Pre.Buildpacks), 1)
				h.AssertEq(t, system.Pre.Buildpacks[0].ID, "pre-only")
				h.AssertEq(t, len(system.Post.Buildpacks), 0)
			})
		})

		when("system.toml exists with only post buildpacks", func() {
			it("returns only post buildpacks", func() {
				systemTOMLContents := `
[system]
  [system.post]
    [[system.post.buildpacks]]
      id = "post-only"
      version = "3.0.0"
`
				h.Mkfile(t, systemTOMLContents, filepath.Join(tmpDir, "system.toml"))

				handler := files.NewHandler()
				system, err := handler.ReadSystem(filepath.Join(tmpDir, "system.toml"), logger)

				h.AssertNil(t, err)
				h.AssertEq(t, len(system.Pre.Buildpacks), 0)
				h.AssertEq(t, len(system.Post.Buildpacks), 1)
				h.AssertEq(t, system.Post.Buildpacks[0].ID, "post-only")
			})
		})

		when("system.toml does not exist", func() {
			it("returns empty system without error", func() {
				handler := files.NewHandler()
				system, err := handler.ReadSystem(filepath.Join(tmpDir, "nonexistent.toml"), logger)

				h.AssertNil(t, err)
				h.AssertEq(t, len(system.Pre.Buildpacks), 0)
				h.AssertEq(t, len(system.Post.Buildpacks), 0)
			})
		})

		when("system.toml is empty", func() {
			it("returns empty system without error", func() {
				systemTOMLContents := `[system]`
				h.Mkfile(t, systemTOMLContents, filepath.Join(tmpDir, "system.toml"))

				handler := files.NewHandler()
				system, err := handler.ReadSystem(filepath.Join(tmpDir, "system.toml"), logger)

				h.AssertNil(t, err)
				h.AssertEq(t, len(system.Pre.Buildpacks), 0)
				h.AssertEq(t, len(system.Post.Buildpacks), 0)
			})
		})
	})
}
