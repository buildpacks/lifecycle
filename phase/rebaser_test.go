package phase_test

import (
	"encoding/json"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRebaser(t *testing.T) {
	spec.Run(t, "Rebaser", testRebaser, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRebaser(t *testing.T, when spec.G, it spec.S) {
	var (
		rebaser           *phase.Rebaser
		fakeAppImage      *fakes.Image
		fakeNewBaseImage  *fakes.Image
		fakePreviousImage *fakes.Image
		additionalNames   []string
		md                files.LayersMetadataCompat
		logHandler        *memory.Handler
	)

	it.Before(func() {
		fakeAppImage = fakes.NewImage(
			"some-repo/app-image",
			"some-top-layer-sha",
			local.IDIdentifier{
				ImageID: "some-image-id",
			},
		)
		h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))

		fakeNewBaseImage = fakes.NewImage(
			"some-repo/new-base-image",
			"new-top-layer-sha",
			local.IDIdentifier{
				ImageID: "new-run-id",
			},
		)
		h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))

		fakePreviousImage = fakes.NewImage(
			"some-repo/previous-image",
			"previous-layer-sha",
			local.IDIdentifier{
				ImageID: "previous-run-id",
			},
		)
		h.AssertNil(t, fakePreviousImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))

		lifecycleMD := files.LayersMetadata{
			Stack: &files.Stack{RunImage: files.RunImageForExport{
				Image: fakeNewBaseImage.Name(),
			}},
		}
		label, err := json.Marshal(lifecycleMD)
		h.AssertNil(t, err)
		h.AssertNil(t, fakeAppImage.SetLabel(platform.LifecycleMetadataLabel, string(label)))
		h.AssertNil(t, fakeAppImage.SetEnv(platform.EnvPlatformAPI, api.Platform.Latest().String()))

		additionalNames = []string{"some-repo/app-image:foo", "some-repo/app-image:bar"}

		logHandler = memory.New()

		rebaser = &phase.Rebaser{
			Logger:      &log.Logger{Handler: logHandler},
			PlatformAPI: api.Platform.Latest(),
		}
	})

	it.After(func() {
		h.AssertNil(t, fakeAppImage.Cleanup())
		h.AssertNil(t, fakeNewBaseImage.Cleanup())
	})

	when("#Rebase", func() {
		when("app image and run image exist", func() {
			it("updates the base image of the app image", func() {
				_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
				h.AssertNil(t, err)
				h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
			})

			it("saves to all names", func() {
				_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
				h.AssertNil(t, err)
				h.AssertContains(t, fakeAppImage.SavedNames(), "some-repo/app-image", "some-repo/app-image:foo", "some-repo/app-image:bar")
			})

			it("adds all names to report", func() {
				report, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
				h.AssertNil(t, err)
				h.AssertContains(t, report.Image.Tags, "some-repo/app-image", "some-repo/app-image:foo", "some-repo/app-image:bar")
			})

			it("sets the top layer in the metadata", func() {
				_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
				h.AssertNil(t, err)
				h.AssertNil(t, image.DecodeLabel(fakeAppImage, platform.LifecycleMetadataLabel, &md))

				h.AssertEq(t, md.RunImage.TopLayer, "new-top-layer-sha")
			})

			it("sets the run image reference in the metadata", func() {
				_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
				h.AssertNil(t, err)
				h.AssertNil(t, image.DecodeLabel(fakeAppImage, platform.LifecycleMetadataLabel, &md))

				h.AssertEq(t, md.RunImage.Reference, "new-run-id")
			})

			it("preserves other existing metadata", func() {
				h.AssertNil(t, fakeAppImage.SetLabel(
					platform.LifecycleMetadataLabel,
					`{"app": [{"sha": "123456"}], "buildpacks":[{"key": "buildpack.id", "layers": {}}]}`,
				))
				rebaser.Force = true // skip run image validations
				_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
				h.AssertNil(t, err)
				h.AssertNil(t, image.DecodeLabel(fakeAppImage, platform.LifecycleMetadataLabel, &md))

				h.AssertEq(t, len(md.Buildpacks), 1)
				h.AssertEq(t, md.Buildpacks[0].ID, "buildpack.id")
				h.AssertEq(t, md.App, []any{map[string]any{"sha": "123456"}})
			})

			when("updating run image metadata", func() {
				it.Before(func() {
					lifecycleMD := files.LayersMetadata{
						RunImage: files.RunImageForRebase{
							TopLayer:  "some-top-layer",
							Reference: "some-run-image-digest-reference",
							RunImageForExport: files.RunImageForExport{
								Image:   "some-run-image-tag-reference",
								Mirrors: []string{"some-run-image-mirror"},
							},
						},
						Stack: &files.Stack{RunImage: files.RunImageForExport{
							Image:   "some-run-image-tag-reference",
							Mirrors: []string{"some-run-image-mirror"},
						}},
					}
					label, err := json.Marshal(lifecycleMD)
					h.AssertNil(t, err)
					h.AssertNil(t, fakeAppImage.SetLabel(platform.LifecycleMetadataLabel, string(label)))
				})

				when("existing run image metadata", func() {
					when("does not include new run image", func() {
						when("force", func() {
							when("true", func() {
								it.Before(func() {
									rebaser.Force = true
								})

								it("warns and overrides the existing metadata", func() {
									_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
									h.AssertNil(t, err)

									assertLogEntry(t, logHandler, `new base image 'some-repo/new-base-image' not found in existing run image metadata: {"topLayer":"new-top-layer-sha","reference":"new-run-id","image":"some-run-image-tag-reference","mirrors":["some-run-image-mirror"]}`)

									h.AssertNil(t, image.DecodeLabel(fakeAppImage, platform.LifecycleMetadataLabel, &md))
									var empty []string
									h.AssertEq(t, md.RunImage.TopLayer, "new-top-layer-sha")
									h.AssertEq(t, md.RunImage.Reference, "new-run-id")
									h.AssertEq(t, md.RunImage.Image, "some-repo/new-base-image")
									h.AssertEq(t, md.RunImage.Mirrors, empty)
									h.AssertEq(t, md.Stack.RunImage.Image, "some-repo/new-base-image")
									h.AssertEq(t, md.Stack.RunImage.Mirrors, empty)
								})
							})

							when("false", func() {
								it.Before(func() {
									rebaser.Force = false
								})

								it("errors", func() {
									_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
									h.AssertError(t, err, `rebase app image: new base image 'some-repo/new-base-image' not found in existing run image metadata: {"topLayer":"new-top-layer-sha","reference":"new-run-id","image":"some-run-image-tag-reference","mirrors":["some-run-image-mirror"]}`)
								})

								when("tag is different", func() {
									it.Before(func() {
										fakeNewBaseImage = fakes.NewImage(
											"some-run-image-mirror:new-tag",
											"new-top-layer-sha",
											local.IDIdentifier{
												ImageID: "new-run-id",
											},
										)
										h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
									})

									it("doesn't match", func() {
										_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
										h.AssertError(t, err, `rebase app image: new base image 'some-run-image-mirror:new-tag' not found in existing run image metadata: {"topLayer":"new-top-layer-sha","reference":"new-run-id","image":"some-run-image-tag-reference","mirrors":["some-run-image-mirror"]}`)
									})
								})

								when("platform API < 0.12", func() {
									it.Before(func() {
										rebaser.PlatformAPI = api.MustParse("0.11")
									})

									it("preserves the existing metadata", func() {
										_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
										h.AssertNil(t, err)

										h.AssertNil(t, image.DecodeLabel(fakeAppImage, platform.LifecycleMetadataLabel, &md))
										h.AssertEq(t, md.RunImage.TopLayer, "new-top-layer-sha")
										h.AssertEq(t, md.RunImage.Reference, "new-run-id")
										h.AssertEq(t, md.RunImage.Image, "some-run-image-tag-reference")
										h.AssertEq(t, md.RunImage.Mirrors, []string{"some-run-image-mirror"})
										h.AssertEq(t, md.Stack.RunImage.Image, "some-run-image-tag-reference")
										h.AssertEq(t, md.Stack.RunImage.Mirrors, []string{"some-run-image-mirror"})
									})
								})
							})
						})
					})

					when("includes new run image", func() {
						it.Before(func() {
							fakeNewBaseImage = fakes.NewImage(
								"some-run-image-mirror",
								"new-top-layer-sha",
								local.IDIdentifier{
									ImageID: "new-run-id",
								},
							)
							h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
						})

						it("preserves the existing metadata", func() {
							_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
							h.AssertNil(t, err)

							h.AssertNil(t, image.DecodeLabel(fakeAppImage, platform.LifecycleMetadataLabel, &md))
							h.AssertEq(t, md.RunImage.TopLayer, "new-top-layer-sha")
							h.AssertEq(t, md.RunImage.Reference, "new-run-id")
							h.AssertEq(t, md.RunImage.Image, "some-run-image-tag-reference")
							h.AssertEq(t, md.RunImage.Mirrors, []string{"some-run-image-mirror"})
							h.AssertEq(t, md.Stack.RunImage.Image, "some-run-image-tag-reference")
							h.AssertEq(t, md.Stack.RunImage.Mirrors, []string{"some-run-image-mirror"})
						})

						when("reference includes docker registry", func() {
							it.Before(func() {
								fakeNewBaseImage = fakes.NewImage(
									"index.docker.io/some-run-image-mirror",
									"new-top-layer-sha",
									local.IDIdentifier{
										ImageID: "new-run-id",
									},
								)
								h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
							})

							it("still matches", func() {
								_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
								h.AssertNil(t, err)

								h.AssertNil(t, image.DecodeLabel(fakeAppImage, platform.LifecycleMetadataLabel, &md))
								h.AssertEq(t, md.RunImage.TopLayer, "new-top-layer-sha")
								h.AssertEq(t, md.RunImage.Reference, "new-run-id")
								h.AssertEq(t, md.RunImage.Image, "some-run-image-tag-reference")
								h.AssertEq(t, md.RunImage.Mirrors, []string{"some-run-image-mirror"})
								h.AssertEq(t, md.Stack.RunImage.Image, "some-run-image-tag-reference")
								h.AssertEq(t, md.Stack.RunImage.Mirrors, []string{"some-run-image-mirror"})
							})
						})
					})

					when("includes new run image as mirror", func() {
						it.Before(func() {
							fakeNewBaseImage = fakes.NewImage(
								"some-run-image-mirror",
								"new-top-layer-sha",
								local.IDIdentifier{
									ImageID: "new-run-id",
								},
							)
							h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
						})

						it("preserves the existing metadata", func() {
							_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
							h.AssertNil(t, err)

							h.AssertNil(t, image.DecodeLabel(fakeAppImage, platform.LifecycleMetadataLabel, &md))
							h.AssertEq(t, md.RunImage.TopLayer, "new-top-layer-sha")
							h.AssertEq(t, md.RunImage.Reference, "new-run-id")
							h.AssertEq(t, md.RunImage.Image, "some-run-image-tag-reference")
							h.AssertEq(t, md.RunImage.Mirrors, []string{"some-run-image-mirror"})
							h.AssertEq(t, md.Stack.RunImage.Image, "some-run-image-tag-reference")
							h.AssertEq(t, md.Stack.RunImage.Mirrors, []string{"some-run-image-mirror"})
						})
					})
				})
			})

			when("image has io.buildpacks.stack.* labels", func() {
				var tests = []struct {
					label         string
					appImageValue string
					runImageValue string
					want          string
				}{
					{"io.buildpacks.stack.distro.name", "ubuntu", "ubuntu", "ubuntu"},
					{"io.buildpacks.stack.changed", "v1", "v2", "v2"},
					{"io.buildpacks.stack.added", "", "new", "new"},
					{"io.buildpacks.stack.removed", "old", "", ""},
					{"io.custom.old", "abc", "", "abc"},
					{"io.custom.stack", "", "def", ""},
				}

				it.Before(func() {
					for _, l := range tests {
						if l.runImageValue != "" {
							h.AssertNil(t, fakeNewBaseImage.SetLabel(l.label, l.runImageValue))
						}
						if l.appImageValue != "" {
							h.AssertNil(t, fakeAppImage.SetLabel(l.label, l.appImageValue))
						}
					}
				})

				it("syncs matching labels", func() {
					_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
					h.AssertNil(t, err)

					for _, test := range tests {
						t.Run(test.label, func(t *testing.T) {
							actual, err := fakeAppImage.Label(test.label)
							h.AssertNil(t, err)
							h.AssertEq(t, test.want, actual)
						})
					}
				})
			})

			when("image has io.buildpacks.base.* labels", func() {
				var tests = []struct {
					label         string
					appImageValue string
					runImageValue string
					want          string
				}{
					{"io.buildpacks.base.homepage", "v1", "v2", "v2"},
					{"io.buildpacks.stack.added", "", "new", "new"},
					{"io.buildpacks.base.removed", "old", "", ""},
				}

				it.Before(func() {
					for _, l := range tests {
						if l.runImageValue != "" {
							h.AssertNil(t, fakeNewBaseImage.SetLabel(l.label, l.runImageValue))
						}
						if l.appImageValue != "" {
							h.AssertNil(t, fakeAppImage.SetLabel(l.label, l.appImageValue))
						}
					}
				})

				it("syncs matching labels", func() {
					_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
					h.AssertNil(t, err)

					for _, test := range tests {
						t.Run(test.label, func(t *testing.T) {
							actual, err := fakeAppImage.Label(test.label)
							h.AssertNil(t, err)
							h.AssertEq(t, test.want, actual)
						})
					}
				})
			})

			when("report.toml", func() {
				when("image has a digest identifier", func() {
					var fakeRemoteDigest = "sha256:c27a27006b74a056bed5d9edcebc394783880abe8691a8c87c78b7cffa6fa5ad"

					it.Before(func() {
						digestRef, err := name.NewDigest("some-repo/app-image@" + fakeRemoteDigest)
						h.AssertNil(t, err)
						fakeAppImage.SetIdentifier(remote.DigestIdentifier{
							Digest: digestRef,
						})
					})

					it("add the digest to the report", func() {
						report, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)

						h.AssertEq(t, report.Image.Digest, fakeRemoteDigest)
					})
				})

				when("checking the image manifest", func() {
					var fakeRemoteManifestSize int64

					when("image has a manifest", func() {
						it.Before(func() {
							fakeRemoteManifestSize = 12345
							fakeAppImage.SetManifestSize(fakeRemoteManifestSize)
						})

						it("add the manifest size to the report", func() {
							report, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
							h.AssertNil(t, err)

							h.AssertEq(t, report.Image.ManifestSize, fakeRemoteManifestSize)
						})
					})

					when("image doesn't have a manifest", func() {
						it.Before(func() {
							fakeRemoteManifestSize = 0
							fakeAppImage.SetManifestSize(fakeRemoteManifestSize)
						})

						it("doesn't set the manifest size in the report.toml", func() {
							report, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
							h.AssertNil(t, err)

							h.AssertEq(t, report.Image.ManifestSize, int64(0))
						})
					})
				})

				when("image has an ID identifier", func() {
					it("add the imageID to the report", func() {
						report, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)

						h.AssertEq(t, report.Image.ImageID, "some-image-id")
					})
				})
			})
		})

		when("validating rebasable", func() {
			when("rebasable label is false", func() {
				it.Before(func() {
					h.AssertNil(t, fakeAppImage.SetLabel(platform.RebasableLabel, "false"))
				})

				when("force", func() {
					when("false", func() {
						it.Before(func() {
							rebaser.Force = false
						})

						it("errors", func() {
							_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
							h.AssertError(t, err, "app image is not marked as rebasable")
						})
					})

					when("true", func() {
						it.Before(func() {
							rebaser.Force = true
						})

						it("warns and allows rebase", func() {
							_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
							h.AssertNil(t, err)

							assertLogEntry(t, logHandler, "app image is not marked as rebasable")
						})
					})
				})
			})

			when("rebasable label is not false", func() {
				it.Before(func() {
					h.AssertNil(t, fakeAppImage.SetLabel(platform.RebasableLabel, "true"))
				})

				it("allows rebase", func() {
					_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
					h.AssertNil(t, err)
				})
			})

			when("rebasable label is empty", func() {
				it.Before(func() {
					h.AssertNil(t, fakeAppImage.SetLabel(platform.RebasableLabel, ""))
				})

				it("allows rebase", func() {
					_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
					h.AssertNil(t, err)
				})
			})

			when("previous image was built using platform API < 0.12", func() {
				it.Before(func() {
					h.AssertNil(t, fakeAppImage.SetEnv(platform.EnvPlatformAPI, "0.11"))
				})

				when("rebasable label is false", func() {
					it.Before(func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.RebasableLabel, "false"))
					})

					it("allows rebase", func() {
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
					})
				})
			})
		})

		when("validating mixins", func() {
			when("mixins are missing on the run image", func() {
				it("allows rebase", func() {
					h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"run:mixin-2\"]"))
					h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.MixinsLabel, "[\"run:mixin-2\"]"))
					_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
					h.AssertNil(t, err)
					h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
				})
			})

			when("previous image was built using platform API < 0.12", func() {
				it.Before(func() {
					h.AssertNil(t, fakeAppImage.SetEnv(platform.EnvPlatformAPI, "0.11"))
				})

				when("there are no mixin labels", func() {
					it("allows rebase", func() {
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("there are invalid mixin labels", func() {
					it("errors", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "thisisn'tvalid!"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertError(t, err, "get app image mixins: failed to unmarshal context of label 'io.buildpacks.stack.mixins': invalid character 'h' in literal true (expecting 'r')")
					})
				})

				when("mixins are not present in either image", func() {
					it("allows rebase", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "null"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.MixinsLabel, "null"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("mixins are not present on the app image", func() {
					it("allows rebase", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "null"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\"]"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("mixins match on the app and run images", func() {
					it("allows rebase", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"run:mixin-2\"]"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"run:mixin-2\"]"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("extra mixins are present on the run image", func() {
					it("allows rebase", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"run:mixin-2\"]"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"run:mixin-2\", \"mixin-3\"]"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("mixins on the app image have stage specifiers that do not match the run image", func() {
					it("allows rebase", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"run:mixin-2\"]"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"mixin-2\"]"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("mixins on the run image have stage specifiers that do not match the app image", func() {
					it("allows rebase", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"run:mixin-2\"]"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.MixinsLabel, "[\"run:mixin-1\", \"run:mixin-2\"]"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("mixins are missing on the run image", func() {
					it("does not allow rebase", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.MixinsLabel, "[\"mixin-1\", \"run:mixin-2\"]"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.MixinsLabel, "[\"run:mixin-2\"]"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertError(t, err, "missing required mixin(s): mixin-1")
					})
				})
			})
		})

		when("validating targets and stacks", func() {
			it.Before(func() {
				rebaser.Force = false
			})

			when("previous image was built using unknown platform API", func() {
				it.Before(func() {
					h.AssertNil(t, fakeAppImage.SetEnv(platform.EnvPlatformAPI, ""))
				})

				when("targets are different", func() {
					it("allows rebase with missing labels", func() {
						h.AssertNil(t, fakeAppImage.SetOS(""))
						h.AssertNil(t, fakeNewBaseImage.SetOS("linux"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})

					it("allows rebase with mismatched variants", func() {
						h.AssertNil(t, fakeAppImage.SetVariant("variant1"))
						h.AssertNil(t, fakeNewBaseImage.SetVariant("variant2"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("stacks are different", func() {
					it("errors and prevents the rebase from taking place when the stacks are different", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertError(t, err, "incompatible stack: 'io.buildpacks.stacks.cflinuxfs3' is not compatible with 'io.buildpacks.stacks.bionic'")
					})

					it("errors and prevents the rebase from taking place when the new base image has no stack defined", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, ""))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertError(t, err, "stack not defined on new base image")
					})

					it("errors and prevents the rebase from taking place when the app image has no stack defined", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, ""))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertError(t, err, "stack not defined on app image")
					})
				})
			})

			when("previous image was built using platform API < 0.12", func() {
				it.Before(func() {
					h.AssertNil(t, fakeAppImage.SetEnv(platform.EnvPlatformAPI, "0.11"))
				})

				when("targets are different", func() {
					it("allows rebase with missing labels", func() {
						h.AssertNil(t, fakeAppImage.SetOS(""))
						h.AssertNil(t, fakeNewBaseImage.SetOS("linux"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})

					it("allows rebase with mismatched variants", func() {
						h.AssertNil(t, fakeAppImage.SetVariant("variant1"))
						h.AssertNil(t, fakeNewBaseImage.SetVariant("variant2"))
						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})

				when("stacks are different", func() {
					it("errors and prevents the rebase from taking place when the stacks are different", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertError(t, err, "incompatible stack: 'io.buildpacks.stacks.cflinuxfs3' is not compatible with 'io.buildpacks.stacks.bionic'")
					})

					it("errors and prevents the rebase from taking place when the new base image has no stack defined", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, ""))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertError(t, err, "stack not defined on new base image")
					})

					it("errors and prevents the rebase from taking place when the app image has no stack defined", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, ""))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertError(t, err, "stack not defined on app image")
					})
				})
			})

			when("previous image was built using platform API >= 0.12", func() {
				when("targets are different", func() {
					when("force", func() {
						when("false", func() {
							it.Before(func() {
								rebaser.Force = false
							})

							it("errors and prevents the rebase from taking place when the os are different", func() {
								h.AssertNil(t, fakeAppImage.SetOS("linux"))
								h.AssertNil(t, fakeNewBaseImage.SetOS("notlinux"))

								_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
								h.AssertError(t, err, `unable to satisfy target os/arch constraints; new run image: {"os":"notlinux","arch":"amd64"}, old run image: {"os":"linux","arch":"amd64"}`)
							})

							it("errors and prevents the rebase from taking place when the architecture are different", func() {
								h.AssertNil(t, fakeAppImage.SetArchitecture("amd64"))
								h.AssertNil(t, fakeNewBaseImage.SetArchitecture("arm64"))

								_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
								h.AssertError(t, err, `unable to satisfy target os/arch constraints; new run image: {"os":"linux","arch":"arm64"}, old run image: {"os":"linux","arch":"amd64"}`)
							})

							it("errors and prevents the rebase from taking place when the architecture variant are different", func() {
								h.AssertNil(t, fakeAppImage.SetVariant("variant1"))
								h.AssertNil(t, fakeNewBaseImage.SetVariant("variant2"))

								_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
								h.AssertError(t, err, `unable to satisfy target os/arch constraints; new run image: {"os":"linux","arch":"amd64","arch-variant":"variant2"}, old run image: {"os":"linux","arch":"amd64","arch-variant":"variant1"}`)
							})

							it("errors and prevents the rebase from taking place when the io.buildpacks.base.distro.name are different", func() {
								h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.base.distro.name", "distro1"))
								h.AssertNil(t, fakeNewBaseImage.SetLabel("io.buildpacks.base.distro.name", "distro2"))

								_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
								h.AssertError(t, err, `unable to satisfy target os/arch constraints; new run image: {"os":"linux","arch":"amd64","distro":{"name":"distro2","version":""}}, old run image: {"os":"linux","arch":"amd64","distro":{"name":"distro1","version":""}}`)
							})

							it("errors and prevents the rebase from taking place when the io.buildpacks.base.distro.version are different", func() {
								h.AssertNil(t, fakeAppImage.SetLabel("io.buildpacks.base.distro.version", "version1"))
								h.AssertNil(t, fakeNewBaseImage.SetLabel("io.buildpacks.base.distro.version", "version2"))

								_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
								h.AssertError(t, err, `unable to satisfy target os/arch constraints; new run image: {"os":"linux","arch":"amd64","distro":{"name":"","version":"version2"}}, old run image: {"os":"linux","arch":"amd64","distro":{"name":"","version":"version1"}}`)
							})
						})
					})

					when("true", func() {
						it.Before(func() {
							rebaser.Force = true
						})

						it("warns and allows rebase when the os are different", func() {
							h.AssertNil(t, fakeAppImage.SetOS("linux"))
							h.AssertNil(t, fakeNewBaseImage.SetOS("notlinux"))

							_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
							h.AssertNil(t, err)

							assertLogEntry(t, logHandler, `unable to satisfy target os/arch constraints; new run image: {"os":"notlinux","arch":"amd64"}, old run image: {"os":"linux","arch":"amd64"}`)
						})
					})
				})

				when("stacks are different", func() {
					it("allows rebase when the stacks are different", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})

					it("allows rebase when the new base image has no stack defined", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.bionic"))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, ""))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})

					it("allows rebase when the app image has no stack defined", func() {
						h.AssertNil(t, fakeAppImage.SetLabel(platform.StackIDLabel, ""))
						h.AssertNil(t, fakeNewBaseImage.SetLabel(platform.StackIDLabel, "io.buildpacks.stacks.cflinuxfs3"))

						_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, fakeAppImage.Name(), additionalNames)
						h.AssertNil(t, err)
						h.AssertEq(t, fakeAppImage.Base(), "some-repo/new-base-image")
					})
				})
			})
		})

		when("outputImageRef is different than workingImage name", func() {
			var outputImageRef = "fizz"

			it("saves using outputImageRef, not the app image name", func() {
				_, err := rebaser.Rebase(fakeAppImage, fakeNewBaseImage, outputImageRef, additionalNames)
				h.AssertNil(t, err)
				h.AssertContains(t, fakeAppImage.SavedNames(), append(additionalNames, outputImageRef)...)
				h.AssertDoesNotContain(t, fakeAppImage.SavedNames(), fakePreviousImage.Name())
			})
		})
	})
}
