package platform_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestLifecycleInputs(t *testing.T) {
	spec.Run(t, "LifecycleInputs", testLifecycleInputs, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testLifecycleInputs(t *testing.T, when spec.G, it spec.S) {
	when("#UpdatePlaceholderPaths", func() {
		when("blank", func() {
			it("does nothing", func() {
				i := &platform.LifecycleInputs{
					AnalyzedPath: "",
					LayersDir:    "some-layers-dir",
					PlatformAPI:  api.Platform.Latest(),
				}
				h.AssertNil(t, platform.UpdatePlaceholderPaths(i, nil))
				h.AssertEq(t, i.AnalyzedPath, "")
			})
		})

		when("order.toml", func() {
			when("at layers directory", func() {
				when("exists", func() {
					var tmpDir string

					it.Before(func() {
						var err error
						tmpDir, err = os.MkdirTemp("", "lifecycle")
						h.AssertNil(t, err)
					})

					it.After(func() {
						os.RemoveAll(tmpDir)
					})

					it("uses order.toml at layers directory", func() {
						h.Mkfile(t, "", filepath.Join(tmpDir, "order.toml"))
						i := &platform.LifecycleInputs{
							OrderPath:   filepath.Join("<layers>", "order.toml"),
							LayersDir:   tmpDir,
							PlatformAPI: api.Platform.Latest(),
						}
						h.AssertNil(t, platform.UpdatePlaceholderPaths(i, nil))
						h.AssertEq(t, i.OrderPath, filepath.Join(tmpDir, "order.toml"))
					})
				})

				when("not exists", func() {
					it("falls back to /cnb/order.toml", func() {
						i := &platform.LifecycleInputs{
							OrderPath:   filepath.Join("<layers>", "order.toml"),
							LayersDir:   "some-layers-dir",
							PlatformAPI: api.Platform.Latest(),
						}
						h.AssertNil(t, platform.UpdatePlaceholderPaths(i, nil))
						h.AssertEq(t, i.OrderPath, platform.DefaultOrderPath)
					})
				})
			})
		})

		when("placeholders", func() {
			it("directory is the layers directory", func() {
				i := &platform.LifecycleInputs{
					AnalyzedPath: filepath.Join("<layers>", "analyzed.toml"),
					LayersDir:    "some-layers-dir",
					PlatformAPI:  api.Platform.Latest(),
				}
				h.AssertNil(t, platform.UpdatePlaceholderPaths(i, nil))
				h.AssertEq(t, i.AnalyzedPath, filepath.Join("some-layers-dir", "analyzed.toml"))
			})

			when("Platform API < 0.5", func() {
				it("directory is the working directory", func() {
					i := &platform.LifecycleInputs{
						AnalyzedPath: filepath.Join("<layers>", "analyzed.toml"),
						LayersDir:    "some-layers-dir",
						PlatformAPI:  api.MustParse("0.4"),
					}
					h.AssertNil(t, platform.UpdatePlaceholderPaths(i, nil))
					h.AssertEq(t, i.AnalyzedPath, "analyzed.toml")
				})
			})

			when("layers is unset", func() {
				it("directory is the working directory", func() {
					i := &platform.LifecycleInputs{
						AnalyzedPath: filepath.Join("<layers>", "analyzed.toml"),
						LayersDir:    "",
						PlatformAPI:  api.Platform.Latest(),
					}
					h.AssertNil(t, platform.UpdatePlaceholderPaths(i, nil))
					h.AssertEq(t, i.AnalyzedPath, "analyzed.toml")
				})
			})
		})

		when("not placeholders", func() {
			it("uses the path that was provided", func() {
				i := &platform.LifecycleInputs{
					AnalyzedPath: "some-path",
					LayersDir:    "some-layers-dir",
					PlatformAPI:  api.Platform.Latest(),
				}
				h.AssertNil(t, platform.UpdatePlaceholderPaths(i, nil))
				h.AssertEq(t, i.AnalyzedPath, "some-path")
			})
		})
	})

	when("#ValidateSameRegistry", func() {
		when("multiple registries are provided", func() {
			it("errors as unsupported", func() {
				err := platform.ValidateSameRegistry("some/repo", "gcr.io/other-repo:latest", "example.com/final-repo")
				h.AssertError(t, err, "writing to multiple registries is unsupported")
			})
		})

		when("a single registry is provided", func() {
			it("does not return an error", func() {
				err := platform.ValidateSameRegistry("gcr.io/some/repo", "gcr.io/other-repo:latest", "gcr.io/final-repo")
				h.AssertNil(t, err)
			})
		})

		when("the tag reference is invalid", func() {
			it("errors", func() {
				err := platform.ValidateSameRegistry("some/Repo")
				h.AssertError(t, err, "could not parse reference: some/Repo")
			})
		})
	})
}
