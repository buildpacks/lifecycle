package patch_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/patch"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestMatcher(t *testing.T) {
	spec.Run(t, "Matcher", testMatcher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testMatcher(t *testing.T, when spec.G, it spec.S) {
	var matcher *patch.LayerMatcher

	it.Before(func() {
		matcher = patch.NewLayerMatcher()
	})

	when("FindMatchingLayers", func() {
		when("exact buildpack and layer match", func() {
			it("finds the matching layer", func() {
				metadata := files.LayersMetadataCompat{
					Buildpacks: []buildpack.LayersMetadata{
						{
							ID: "buildpack-a",
							Layers: map[string]buildpack.LayerMetadata{
								"layer1": {SHA: "sha1"},
								"layer2": {SHA: "sha2"},
							},
						},
					},
				}
				layerPatch := files.LayerPatch{
					Buildpack: "buildpack-a",
					Layer:     "layer1",
				}

				results := matcher.FindMatchingLayers(metadata, layerPatch)

				h.AssertEq(t, len(results), 1)
				h.AssertEq(t, results[0].BuildpackID, "buildpack-a")
				h.AssertEq(t, results[0].LayerName, "layer1")
				h.AssertEq(t, results[0].LayerMetadata.SHA, "sha1")
			})
		})

		when("glob pattern matches buildpack", func() {
			it("finds all matching buildpacks", func() {
				metadata := files.LayersMetadataCompat{
					Buildpacks: []buildpack.LayersMetadata{
						{
							ID: "org/buildpack-java",
							Layers: map[string]buildpack.LayerMetadata{
								"jre": {SHA: "sha1"},
							},
						},
						{
							ID: "org/buildpack-python",
							Layers: map[string]buildpack.LayerMetadata{
								"python": {SHA: "sha2"},
							},
						},
					},
				}
				layerPatch := files.LayerPatch{
					Buildpack: "org/buildpack-*",
					Layer:     "*",
				}

				results := matcher.FindMatchingLayers(metadata, layerPatch)

				h.AssertEq(t, len(results), 2)
			})
		})

		when("glob pattern matches layer name", func() {
			it("finds all matching layers", func() {
				metadata := files.LayersMetadataCompat{
					Buildpacks: []buildpack.LayersMetadata{
						{
							ID: "buildpack-a",
							Layers: map[string]buildpack.LayerMetadata{
								"jre-11": {SHA: "sha1"},
								"jre-17": {SHA: "sha2"},
								"other":  {SHA: "sha3"},
							},
						},
					},
				}
				layerPatch := files.LayerPatch{
					Buildpack: "buildpack-a",
					Layer:     "jre-*",
				}

				results := matcher.FindMatchingLayers(metadata, layerPatch)

				h.AssertEq(t, len(results), 2)
			})
		})

		when("data selectors are provided", func() {
			when("data matches", func() {
				it("includes the layer", func() {
					metadata := files.LayersMetadataCompat{
						Buildpacks: []buildpack.LayersMetadata{
							{
								ID: "buildpack-a",
								Layers: map[string]buildpack.LayerMetadata{
									"jre": {
										SHA: "sha1",
										LayerMetadataFile: buildpack.LayerMetadataFile{
											Data: map[string]interface{}{
												"artifact": map[string]interface{}{
													"version": "17.0.1",
												},
											},
										},
									},
								},
							},
						},
					}
					layerPatch := files.LayerPatch{
						Buildpack: "buildpack-a",
						Layer:     "jre",
						Data: map[string]string{
							"artifact.version": "17.*",
						},
					}

					results := matcher.FindMatchingLayers(metadata, layerPatch)

					h.AssertEq(t, len(results), 1)
				})
			})

			when("data does not match", func() {
				it("excludes the layer", func() {
					metadata := files.LayersMetadataCompat{
						Buildpacks: []buildpack.LayersMetadata{
							{
								ID: "buildpack-a",
								Layers: map[string]buildpack.LayerMetadata{
									"jre": {
										SHA: "sha1",
										LayerMetadataFile: buildpack.LayerMetadataFile{
											Data: map[string]interface{}{
												"artifact": map[string]interface{}{
													"version": "11.0.1",
												},
											},
										},
									},
								},
							},
						},
					}
					layerPatch := files.LayerPatch{
						Buildpack: "buildpack-a",
						Layer:     "jre",
						Data: map[string]string{
							"artifact.version": "17.*",
						},
					}

					results := matcher.FindMatchingLayers(metadata, layerPatch)

					h.AssertEq(t, len(results), 0)
				})
			})

			when("data path does not exist", func() {
				it("excludes the layer", func() {
					metadata := files.LayersMetadataCompat{
						Buildpacks: []buildpack.LayersMetadata{
							{
								ID: "buildpack-a",
								Layers: map[string]buildpack.LayerMetadata{
									"jre": {
										SHA: "sha1",
										LayerMetadataFile: buildpack.LayerMetadataFile{
											Data: map[string]interface{}{
												"other": "value",
											},
										},
									},
								},
							},
						},
					}
					layerPatch := files.LayerPatch{
						Buildpack: "buildpack-a",
						Layer:     "jre",
						Data: map[string]string{
							"artifact.version": "17.*",
						},
					}

					results := matcher.FindMatchingLayers(metadata, layerPatch)

					h.AssertEq(t, len(results), 0)
				})
			})
		})

		when("no layers match", func() {
			it("returns empty results", func() {
				metadata := files.LayersMetadataCompat{
					Buildpacks: []buildpack.LayersMetadata{
						{
							ID: "buildpack-a",
							Layers: map[string]buildpack.LayerMetadata{
								"layer1": {SHA: "sha1"},
							},
						},
					},
				}
				layerPatch := files.LayerPatch{
					Buildpack: "buildpack-b",
					Layer:     "layer1",
				}

				results := matcher.FindMatchingLayers(metadata, layerPatch)

				h.AssertEq(t, len(results), 0)
			})
		})

		when("multiple buildpacks have the same ID", func() {
			it("matches all occurrences", func() {
				metadata := files.LayersMetadataCompat{
					Buildpacks: []buildpack.LayersMetadata{
						{
							ID: "buildpack-a",
							Layers: map[string]buildpack.LayerMetadata{
								"layer1": {SHA: "sha1"},
							},
						},
						{
							ID: "buildpack-a",
							Layers: map[string]buildpack.LayerMetadata{
								"layer1": {SHA: "sha2"},
							},
						},
					},
				}
				layerPatch := files.LayerPatch{
					Buildpack: "buildpack-a",
					Layer:     "layer1",
				}

				results := matcher.FindMatchingLayers(metadata, layerPatch)

				h.AssertEq(t, len(results), 2)
				h.AssertEq(t, results[0].BuildpackIndex, 0)
				h.AssertEq(t, results[1].BuildpackIndex, 1)
			})
		})
	})
}
