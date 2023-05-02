package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/platform"
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
			LayersDir:   filepath.Join("testdata", "layers"),
			PlatformAPI: api.Platform.Latest(),
			RunImageRef: "some-run-image-ref",
			RunPath:     filepath.Join("testdata", "layers", "run.toml"),
			StackPath:   filepath.Join("testdata", "layers", "stack.toml"),
		}

		when("run.toml", func() {
			when("not exists", func() {
				inputs.RunPath = "foo"

				it("returns empty info", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, platform.RunImageForExport{})
				})
			})

			when("contains no images", func() {
				inputs.RunPath = filepath.Join("testdata", "layers", "empty-run.toml")

				it("returns empty info", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, platform.RunImageForExport{})
				})
			})

			when("contains an image matching run image ref", func() {
				inputs.RunImageRef = "some-run-image-from-run-toml"

				it("returns the image", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, platform.RunImageForExport{
						Image:   "some-run-image-from-run-toml",
						Mirrors: []string{"some-run-image-mirror-from-run-toml", "some-other-run-image-mirror-from-run-toml"},
					})
				})
			})

			when("contains an image mirror matching run image ref", func() {
				inputs.RunImageRef = "some-other-run-image-mirror-from-run-toml"

				it("returns the image", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, platform.RunImageForExport{
						Image:   "some-run-image-from-run-toml",
						Mirrors: []string{"some-run-image-mirror-from-run-toml", "some-other-run-image-mirror-from-run-toml"},
					})
				})
			})

			when("contains no image or image mirror matching run image ref", func() {
				it("returns the first image in run.toml", func() {
					result, err := platform.GetRunImageForExport(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, result, platform.RunImageForExport{
						Image:   "some-run-image-from-run-toml",
						Mirrors: []string{"some-run-image-mirror-from-run-toml", "some-other-run-image-mirror-from-run-toml"},
					})
				})

				when("there are extensions", func() {
					inputs.LayersDir = filepath.Join("testdata", "other-layers")

					it("returns the run image ref", func() {
						result, err := platform.GetRunImageForExport(inputs)
						h.AssertNil(t, err)
						h.AssertEq(t, result, platform.RunImageForExport{Image: "some-run-image-ref"})
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
					h.AssertEq(t, result, platform.RunImageForExport{
						Image:   "some-run-image-from-stack-toml",
						Mirrors: []string{"some-run-image-mirror-from-stack-toml", "some-other-run-image-mirror-from-stack-toml"},
					})
				})

				when("not exists", func() {
					inputs.StackPath = "foo"

					it("returns empty info", func() {
						result, err := platform.GetRunImageForExport(inputs)
						h.AssertNil(t, err)
						h.AssertEq(t, result, platform.RunImageForExport{})
					})
				})

				when("contains no images", func() {
					inputs.StackPath = filepath.Join("testdata", "layers", "empty-run.toml")

					it("returns empty info", func() {
						result, err := platform.GetRunImageForExport(inputs)
						h.AssertNil(t, err)
						h.AssertEq(t, result, platform.RunImageForExport{})
					})
				})
			})
		})
	})

	when("we want to get EnvVarsFor a platform.TargetMetadata", func() {
		it("returns the right thing", func() {
			tm := platform.TargetMetadata{Arch: "pentium", ArchVariant: "mmx", ID: "my-id", OS: "linux", Distribution: &platform.OSDistribution{Name: "nix", Version: "22.11"}}
			observed := platform.EnvVarsFor(&tm)
			h.AssertContains(t, observed, "CNB_TARGET_ARCH="+tm.Arch)
			h.AssertContains(t, observed, "CNB_TARGET_VARIANT="+tm.ArchVariant)
			h.AssertContains(t, observed, "CNB_TARGET_DISTRO_NAME="+tm.Distribution.Name)
			h.AssertContains(t, observed, "CNB_TARGET_DISTRO_VERSION="+tm.Distribution.Version)
			h.AssertContains(t, observed, "CNB_TARGET_ID="+tm.ID)
			h.AssertContains(t, observed, "CNB_TARGET_OS="+tm.OS)
			h.AssertEq(t, len(observed), 6)
		})
		it("does not return the wrong thing", func() {
			tm := platform.TargetMetadata{Arch: "pentium", OS: "linux"}
			observed := platform.EnvVarsFor(&tm)
			h.AssertContains(t, observed, "CNB_TARGET_ARCH="+tm.Arch)
			h.AssertContains(t, observed, "CNB_TARGET_OS="+tm.OS)
			// note: per the spec only the ID field is optional, so I guess the others should always be set: https://github.com/buildpacks/rfcs/blob/main/text/0096-remove-stacks-mixins.md#runtime-metadata
			// the empty ones:
			h.AssertContains(t, observed, "CNB_TARGET_VARIANT=")
			h.AssertContains(t, observed, "CNB_TARGET_DISTRO_NAME=")
			h.AssertContains(t, observed, "CNB_TARGET_DISTRO_VERSION=")
			h.AssertEq(t, len(observed), 5)
		})
	})
}
