package lifecycle_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestStack(t *testing.T) {
	spec.Run(t, "Stack", testStack, spec.Report(report.Terminal{}))
}

func testStack(t *testing.T, when spec.G, it spec.S) {
	when("StackValidator", func() {
		var (
			stackValidator *lifecycle.StackValidator
		)

		it.Before(func() {
			stackValidator = &lifecycle.StackValidator{}
		})

		when("#ValidateMixins", func() {
			when("mixins are satisfied", func() {
				it("returns nil", func() {
					bpDesc := buildpack.Descriptor{
						API: "0.3",
						Buildpack: buildpack.Info{
							Name:    "Buildpack A",
							Version: "v1",
						},
						Stacks: []buildpack.Stack{
							{
								ID: "some-stack-id",
								Mixins: []string{
									"some-unprefixed-mixin",
									"some-other-mixin",
									"build:some-mixin",
									"run:some-mixin",
								},
							},
						},
					}
					analyzed := lifecycle.Analyzed{
						BuildImageStackID: "some-stack-id",
						BuildImageMixins:  []string{"some-unprefixed-mixin", "build:some-other-mixin", "some-mixin"},
						RunImageMixins:    []string{"some-unprefixed-mixin", "run:some-other-mixin", "some-mixin"},
					}

					err := stackValidator.ValidateMixins(bpDesc, analyzed)
					h.AssertNil(t, err)
				})
			})

			when("mixins are not satisfied", func() {
				when("by build image", func() {
					it("returns an error", func() {
						bpDesc := buildpack.Descriptor{
							API: "0.3",
							Buildpack: buildpack.Info{
								Name:    "Buildpack A",
								Version: "v1",
							},
							Stacks: []buildpack.Stack{
								{
									ID:     "some-stack-id",
									Mixins: []string{"some-present-mixin", "build:some-missing-mixin", "run:some-present-mixin"},
								},
							},
						}
						analyzed := lifecycle.Analyzed{
							BuildImageStackID: "some-stack-id",
							BuildImageMixins:  []string{"some-present-mixin"},
							RunImageMixins:    []string{"some-present-mixin"},
						}

						err := stackValidator.ValidateMixins(bpDesc, analyzed)
						h.AssertError(t, err, "buildpack Buildpack A v1 missing required mixin build:some-missing-mixin")
					})
				})

				when("by run image", func() {
					it("returns an error", func() {
						bpDesc := buildpack.Descriptor{
							API: "0.3",
							Buildpack: buildpack.Info{
								Name:    "Buildpack A",
								Version: "v1",
							},
							Stacks: []buildpack.Stack{
								{
									ID:     "some-stack-id",
									Mixins: []string{"some-present-mixin", "build:some-present-mixin", "run:some-missing-mixin"},
								},
							},
						}
						analyzed := lifecycle.Analyzed{
							BuildImageStackID: "some-stack-id",
							BuildImageMixins:  []string{"some-present-mixin"},
							RunImageMixins:    []string{"some-present-mixin"},
						}

						err := stackValidator.ValidateMixins(bpDesc, analyzed)
						h.AssertError(t, err, "buildpack Buildpack A v1 missing required mixin run:some-missing-mixin")
					})
				})
			})
		})
	})
}
