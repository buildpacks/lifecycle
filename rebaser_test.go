package lifecycle_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/name"
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
				_, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertNil(t, err)
				h.AssertEq(t, fakeWorkingImage.Base(), "some-repo/new-base-image")
			})

			it("saves to all names", func() {
				_, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertNil(t, err)
				h.AssertContains(t, fakeWorkingImage.SavedNames(), "some-repo/app-image", "some-repo/app-image:foo", "some-repo/app-image:bar")
			})

			it("adds all names to report", func() {
				report, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertNil(t, err)
				h.AssertContains(t, report.Image.Tags, "some-repo/app-image", "some-repo/app-image:foo", "some-repo/app-image:bar")
			})

			it("sets the top layer in the metadata", func() {
				_, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertNil(t, err)
				h.AssertNil(t, lifecycle.DecodeLabel(fakeWorkingImage, lifecycle.LayerMetadataLabel, &md))

				h.AssertEq(t, md.RunImage.TopLayer, "new-top-layer-sha")
			})

			it("sets the run image reference in the metadata", func() {
				_, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertNil(t, err)
				h.AssertNil(t, lifecycle.DecodeLabel(fakeWorkingImage, lifecycle.LayerMetadataLabel, &md))

				h.AssertEq(t, md.RunImage.Reference, "new-run-id")
			})

			it("preserves other existing metadata", func() {
				h.AssertNil(t, fakeWorkingImage.SetLabel(
					lifecycle.LayerMetadataLabel,
					`{"app": [{"sha": "123456"}], "buildpacks":[{"key": "buildpack.id", "layers": {}}]}`,
				))
				_, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertNil(t, err)
				h.AssertNil(t, lifecycle.DecodeLabel(fakeWorkingImage, lifecycle.LayerMetadataLabel, &md))

				h.AssertEq(t, len(md.Buildpacks), 1)
				h.AssertEq(t, md.Buildpacks[0].ID, "buildpack.id")
				h.AssertEq(t, md.App, []interface{}{map[string]interface{}{"sha": "123456"}})
			})

			when("image has a digest identifier", func() {
				var fakeRemoteDigest = "sha256:c27a27006b74a056bed5d9edcebc394783880abe8691a8c87c78b7cffa6fa5ad"

				it.Before(func() {
					digestRef, err := name.NewDigest("some-repo/app-image@" + fakeRemoteDigest)
					h.AssertNil(t, err)
					fakeWorkingImage.SetIdentifier(remote.DigestIdentifier{
						Digest: digestRef,
					})
				})

				it("add the digest to the report", func() {
					report, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
					h.AssertNil(t, err)

					h.AssertEq(t, report.Image.Digest, fakeRemoteDigest)
				})
			})

			when("image has an ID identifier", func() {
				it("add the imageID to the report", func() {
					report, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
					h.AssertNil(t, err)

					h.AssertEq(t, report.Image.ImageID, "some-image-id")
				})
			})
		})

		when("app image and run image are based on different stacks", func() {
			it("returns an error and prevents the rebase from taking place when the stacks are different", func() {
				h.AssertNil(t, fakeWorkingImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.bionic"))
				h.AssertNil(t, fakeNewBaseImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

				_, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertError(t, err, "incompatible stack: 'io.buildpacks.stacks.cflinuxfs3' is not compatible with 'io.buildpacks.stacks.bionic'")
			})

			it("returns an error and prevents the rebase from taking place when the new base image has no stack defined", func() {
				h.AssertNil(t, fakeWorkingImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.bionic"))
				h.AssertNil(t, fakeNewBaseImage.SetLabel(lifecycle.StackIDLabel, ""))

				_, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertError(t, err, "stack not defined on new base image")
			})

			it("returns an error and prevents the rebase from taking place when the working image has no stack defined", func() {
				h.AssertNil(t, fakeWorkingImage.SetLabel(lifecycle.StackIDLabel, ""))
				h.AssertNil(t, fakeNewBaseImage.SetLabel(lifecycle.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

				_, err := rebaser.Rebase(fakeWorkingImage, fakeNewBaseImage, additionalNames)
				h.AssertError(t, err, "stack not defined on working image")
			})
		})
	})
}
