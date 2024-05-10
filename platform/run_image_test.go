package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
)

func TestRunImage(t *testing.T) {
	spec.Run(t, "RunImage", testRunImage, spec.Report(report.Terminal{}))
}

func testRunImage(t *testing.T, when spec.G, it spec.S) {
	when(".GetRunImageForExport", func() {
		var inputs = platform.LifecycleInputs{
			AnalyzedPath: filepath.Join("testdata", "layers", "analyzed.toml"),
			LayersDir:    filepath.Join("testdata", "layers"),
			PlatformAPI:  api.Platform.Latest(),
			RunPath:      filepath.Join("testdata", "layers", "run.toml"),
			StackPath:    filepath.Join("testdata", "layers", "stack.toml"),
		}

		when("run.toml", func() {
			when("not exists", func() {
				inputs.RunPath = "foo"

				it("returns empty info", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, files.RunImageForExport{})
				})
			})

			when("contains no images", func() {
				inputs.RunPath = filepath.Join("testdata", "layers", "empty-run.toml")

				it("returns empty info", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, files.RunImageForExport{})
				})
			})

			when("contains an image matching run image ref", func() {
				it("returns the image", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, files.RunImageForExport{
						Image:   "some-user-provided-run-image",
						Mirrors: []string{"some-user-provided-run-image-mirror-1", "some-user-provided-run-image-mirror-2"},
					})
				})

				when("reference includes docker registry", func() {
					inputs.AnalyzedPath = filepath.Join("testdata", "layers", "analyzed-docker.toml")

					it("still matches", func() {
						result, err := platform.GetRunImageForExport(inputs)
						h.AssertNil(t, err)
						h.AssertEq(t, result, files.RunImageForExport{
							Image:   "some-user-provided-run-image",
							Mirrors: []string{"some-user-provided-run-image-mirror-1", "some-user-provided-run-image-mirror-2"},
						})
					})
				})
			})

			when("contains an image mirror matching run image ref", func() {
				it("returns the image", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, files.RunImageForExport{
						Image:   "some-user-provided-run-image",
						Mirrors: []string{"some-user-provided-run-image-mirror-1", "some-user-provided-run-image-mirror-2"},
					})
				})
			})

			when("contains no image or image mirror matching run image ref", func() {
				inputs.AnalyzedPath = filepath.Join("testdata", "layers", "analyzed-other.toml")

				it("returns the first image in run.toml", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, files.RunImageForExport{
						Image:   "some-other-user-provided-run-image",
						Mirrors: []string{"some-other-user-provided-run-image-mirror-1", "some-other-user-provided-run-image-mirror-2"},
					})
				})

				when("there are extensions", func() {
					inputs.AnalyzedPath = filepath.Join("testdata", "layers", "analyzed-other.toml")
					inputs.LayersDir = filepath.Join("testdata", "other-layers") // force <layers>/config/metadata.toml

					it("returns the run image ref from analyzed.toml", func() {
						result, err := platform.GetRunImageForExport(inputs)
						h.AssertNil(t, err)
						h.AssertEq(t, result, files.RunImageForExport{Image: "some-new-user-provided-run-image"})
					})
				})
			})
		})

		when("platform api < 0.12", func() {
			inputs.PlatformAPI = api.MustParse("0.11")

			when("stack.toml", func() {
				it("returns the data in stack.toml", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, files.RunImageForExport{
						Image:   "some-other-user-provided-run-image",
						Mirrors: []string{"some-other-user-provided-run-image-mirror-1", "some-other-user-provided-run-image-mirror-2"},
					})
				})

				when("not exists", func() {
					inputs.StackPath = "foo"

					it("returns empty info", func() {
						result, err := platform.GetRunImageForExport(inputs)
						h.AssertNil(t, err)
						h.AssertEq(t, result, files.RunImageForExport{})
					})
				})

				when("contains no images", func() {
					inputs.StackPath = filepath.Join("testdata", "layers", "empty-run.toml")

					it("returns empty info", func() {
						result, err := platform.GetRunImageForExport(inputs)
						h.AssertNil(t, err)
						h.AssertEq(t, result, files.RunImageForExport{})
					})
				})
			})
		})
	})

	when(".GetRunImageFromMetadata", func() {
		var inputs = platform.LifecycleInputs{
			PlatformAPI: api.Platform.Latest(),
		}

		var md = files.LayersMetadata{}

		when("run image not set in metadata", func() {
			it("errors", func() {
				result, err := platform.GetRunImageFromMetadata(inputs, md)
				h.AssertNotNil(t, err)
				h.AssertEq(t, result.Image, "")
			})
		})

		when("run image set in runImage metadata", func() {
			md.RunImage.RunImageForExport.Image = "run-image-in-metadata"

			it("returns the run image from runImage metadata", func() {
				result, err := platform.GetRunImageFromMetadata(inputs, md)
				h.AssertNil(t, err)
				h.AssertEq(t, result, files.RunImageForExport{Image: "run-image-in-metadata"})
			})
		})

		when("run image set in stack metadata", func() {
			md.Stack = &files.Stack{
				RunImage: files.RunImageForExport{
					Image: "run-image-in-stack-metadata",
				},
			}

			it("returns the run image from stack metadata", func() {
				result, err := platform.GetRunImageFromMetadata(inputs, md)
				h.AssertNil(t, err)
				h.AssertEq(t, result, files.RunImageForExport{Image: "run-image-in-stack-metadata"})
			})
		})

		when("platform api < 0.12", func() {
			inputs.PlatformAPI = api.MustParse("0.11")

			when("run image set in runImage metadata", func() {
				md.RunImage.RunImageForExport.Image = "run-image-in-metadata"

				it("fails to return at this platform version", func() {
					result, err := platform.GetRunImageFromMetadata(inputs, md)
					h.AssertNotNil(t, err)
					h.AssertEq(t, result.Image, "")
				})
			})
		})
	})

	when(".BestRunImageMirrorFor", func() {
		var (
			stackMD            *files.Stack
			nopCheckReadAccess = func(_ string, _ authn.Keychain) (bool, error) {
				return true, nil
			}
		)

		it.Before(func() {
			stackMD = &files.Stack{RunImage: files.RunImageForExport{
				Image: "first.com/org/repo",
				Mirrors: []string{
					"myorg/myrepo",
					"zonal.gcr.io/org/repo",
					"gcr.io/org/repo",
				},
			}}
		})

		when("repoName is dockerhub", func() {
			it("returns the dockerhub image", func() {
				name, err := platform.BestRunImageMirrorFor("index.docker.io", stackMD.RunImage, nopCheckReadAccess)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "myorg/myrepo")
			})
		})

		when("registry is gcr.io", func() {
			it("returns the gcr.io image", func() {
				name, err := platform.BestRunImageMirrorFor("gcr.io", stackMD.RunImage, nopCheckReadAccess)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "gcr.io/org/repo")
			})

			when("registry is zonal.gcr.io", func() {
				it("returns the gcr image", func() {
					name, err := platform.BestRunImageMirrorFor("zonal.gcr.io", stackMD.RunImage, nopCheckReadAccess)
					h.AssertNil(t, err)
					h.AssertEq(t, name, "zonal.gcr.io/org/repo")
				})
			})

			when("registry is missingzone.gcr.io", func() {
				it("returns the run image", func() {
					name, err := platform.BestRunImageMirrorFor("missingzone.gcr.io", stackMD.RunImage, nopCheckReadAccess)
					h.AssertNil(t, err)
					h.AssertEq(t, name, "first.com/org/repo")
				})
			})
		})

		when("one of the images is non-parsable", func() {
			it.Before(func() {
				stackMD.RunImage.Mirrors = []string{"as@ohd@as@op", "gcr.io/myorg/myrepo"}
			})

			it("skips over it", func() {
				name, err := platform.BestRunImageMirrorFor("gcr.io", stackMD.RunImage, nopCheckReadAccess)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "gcr.io/myorg/myrepo")
			})
		})

		when("there is no read access", func() {
			it("fails", func() {
				noReadAccess := func(_ string, _ authn.Keychain) (bool, error) {
					return false, nil
				}

				_, err := platform.BestRunImageMirrorFor("gcr.io", stackMD.RunImage, noReadAccess)
				h.AssertNotNil(t, err)
				expected := "failed to find accessible run image"
				h.AssertStringContains(t, err.Error(), expected)
			})
		})
	})
}
