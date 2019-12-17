package lifecycle_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRebaser(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Rebaser", testRebaser, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRebaser(t *testing.T, when spec.G, it spec.S) {
	var (
		rebaser          *lifecycle.Rebaser
		fakeWorkingImage *fakes.Image
		fakeNewBaseImage *fakes.Image
		additionalNames  []string
		md               lifecycle.LayersMetadataCompat
	)

	it.Before(func() {
		fakeWorkingImage = fakes.NewImage(
			"some-repo/app-image",
			"some-top-layer-sha",
			local.IDIdentifier{
				ImageID: "some-image-id",
			},
		)
		h.AssertNil(t, fakeWorkingImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.bionic"))

		fakeNewBaseImage = fakes.NewImage(
			"some-repo/new-base-image",
			"new-top-layer-sha",
			local.IDIdentifier{
				ImageID: "new-run-id",
			},
		)
		h.AssertNil(t, fakeNewBaseImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.bionic"))

		additionalNames = []string{"some-repo/app-image:foo", "some-repo/app-image:bar"}

		rebaser = &lifecycle.Rebaser{
			Logger: &log.Logger{Handler: &discard.Handler{}},
		}
	})

	it.After(func() {
		h.AssertNil(t, fakeWorkingImage.Cleanup())
		h.AssertNil(t, fakeNewBaseImage.Cleanup())
	})

	when("#Rebase", func() {
		when("app image and run image exist", func() {
			it("updates the base image of the working image", func() {
				h.AssertNil(t, rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames))
				h.AssertEq(t, fakeWorkingImage.Base(), "some-repo/new-base-image")
			})

			it("saves to all names", func() {
				h.AssertNil(t, rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames))
				h.AssertContains(t, fakeWorkingImage.SavedNames(), "some-repo/app-image", "some-repo/app-image:foo", "some-repo/app-image:bar")
			})

			it("sets the top layer in the metadata", func() {
				h.AssertNil(t, rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames))
				h.AssertNil(t, lifecycle.DecodeLabel(fakeWorkingImage, lifecycle.LayerMetadataLabel, &md))

				h.AssertEq(t, md.RunImage.TopLayer, "new-top-layer-sha")
			})

			it("sets the run image reference in the metadata", func() {
				h.AssertNil(t, rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames))
				h.AssertNil(t, lifecycle.DecodeLabel(fakeWorkingImage, lifecycle.LayerMetadataLabel, &md))

				h.AssertEq(t, md.RunImage.Reference, "new-run-id")
			})

			it("preserves other existing metadata", func() {
				h.AssertNil(t, fakeWorkingImage.SetLabel(
					lifecycle.LayerMetadataLabel,
					`{"app": [{"sha": "123456"}], "buildpacks":[{"key": "buildpack.id", "layers": {}}]}`,
				))
				h.AssertNil(t, rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames))
				h.AssertNil(t, lifecycle.DecodeLabel(fakeWorkingImage, lifecycle.LayerMetadataLabel, &md))

				h.AssertEq(t, len(md.Buildpacks), 1)
				h.AssertEq(t, md.Buildpacks[0].ID, "buildpack.id")
				h.AssertEq(t, md.App, []interface{}{map[string]interface{}{"sha": "123456"}})
			})
		})

		when("app image and run image are based on different stacks", func() {
			it("returns an error and prevents the rebase from taking place when the stacks are different", func() {
				h.AssertNil(t, fakeWorkingImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.bionic"))
				h.AssertNil(t, fakeNewBaseImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

				err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertError(t, err, "incompatible stack: 'io.buildpacks.stacks.cflinuxfs3' is not compatible with 'io.buildpacks.stacks.bionic'")
			})

			it("returns an error and prevents the rebase from taking place when the new base image has no stack defined", func() {
				h.AssertNil(t, fakeWorkingImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.bionic"))
				h.AssertNil(t, fakeNewBaseImage.SetLabel(lifecycle.StackIDLabel, ""))

				err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertError(t, err, "stack not defined on new base image")
			})

			it("returns an error and prevents the rebase from taking place when the working image has no stack defined", func() {
				h.AssertNil(t, fakeWorkingImage.SetLabel(lifecycle.StackIDLabel, ""))
				h.AssertNil(t, fakeNewBaseImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

				err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertError(t, err, "stack not defined on working image")
			})
		})
	})
}
