package lifecycle_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	specreport "github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestExporter(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Exporter", testExporter, spec.Parallel(), spec.Report(specreport.Terminal{}))
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	var (
		exporter     *lifecycle.Exporter
		tmpDir       string
		mockCtrl     *gomock.Controller
		layerFactory *testmock.MockLayerFactory
		fakeAppImage *fakes.Image
		logHandler   = memory.New()
		opts         = lifecycle.ExportOptions{
			RunImageRef:     "run-image-reference",
			AdditionalNames: []string{},
		}
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		layerFactory = testmock.NewMockLayerFactory(mockCtrl)

		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle.exporter.layer")
		h.AssertNil(t, err)

		launcherPath, err := filepath.Abs(filepath.Join("testdata", "exporter", "launcher"))
		h.AssertNil(t, err)

		opts.LauncherConfig = lifecycle.LauncherConfig{
			Path: launcherPath,
			Metadata: lifecycle.LauncherMetadata{
				Version: "1.2.3",
				Source: lifecycle.SourceMetadata{
					Git: lifecycle.GitMetadata{
						Repository: "github.com/buildpacks/lifecycle",
						Commit:     "asdf1234",
					},
				},
			},
		}

		opts.LayersDir = filepath.Join(tmpDir, "layers")
		h.AssertNil(t, os.Mkdir(opts.LayersDir, 0777))
		h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "launcher"), []byte("some-launcher"), 0777))
		opts.AppDir = filepath.Join(tmpDir, "app")

		fakeAppImage = fakes.NewImage(
			"some-repo/app-image",
			"some-top-layer-sha",
			local.IDIdentifier{
				ImageID: "some-image-id",
			},
		)
		opts.WorkingImage = fakeAppImage

		opts.AdditionalNames = []string{"some-repo/app-image:foo", "some-repo/app-image:bar"}

		// mock LayerFactory returns layer with deterministic characteristic for a give layer it
		h.AssertNil(t, os.Mkdir(filepath.Join(tmpDir, "artifacts"), 0777))
		layerFactory.EXPECT().
			DirLayer(gomock.Any(), gomock.Any()).
			DoAndReturn(func(id string, dir string) (layers.Layer, error) {
				return createTestLayer(id, tmpDir)
			}).AnyTimes()

		layerFactory.EXPECT().
			LauncherLayer(launcherPath).
			DoAndReturn(func(path string) (layers.Layer, error) { return createTestLayer("launcher", tmpDir) }).
			AnyTimes()

		layerFactory.EXPECT().
			ProcessTypesLayer(launch.Metadata{
				Processes: []launch.Process{
					{
						Type:        "some-process-type",
						Command:     "/some/command",
						Args:        []string{"some", "command", "args"},
						Direct:      true,
						BuildpackID: "buildpack.id",
					}},
			}).
			DoAndReturn(func(_ launch.Metadata) (layers.Layer, error) {
				return createTestLayer("process-types", tmpDir)
			}).
			AnyTimes()

		// if there are no slices return a single deterministic app layer
		layerFactory.EXPECT().
			SliceLayers(gomock.Any(), nil).
			DoAndReturn(func(dir string, slices []layers.Slice) ([]layers.Layer, error) {
				if dir != opts.AppDir {
					return nil, fmt.Errorf("SliceLayers received %s but expected %s", dir, opts.AppDir)
				}
				layer, err := createTestLayer("app", tmpDir)
				if err != nil {
					return nil, err
				}
				return []layers.Layer{layer}, nil
			}).AnyTimes()

		exporter = &lifecycle.Exporter{
			Buildpacks: []lifecycle.Buildpack{
				{ID: "buildpack.id", Version: "1.2.3"},
				{ID: "other.buildpack.id", Version: "4.5.6", Optional: false},
			},
			LayerFactory: layerFactory,
			Logger:       &log.Logger{Handler: logHandler},
			PlatformAPI:  api.MustParse("0.4"),
		}
	})

	it.After(func() {
		h.AssertNil(t, fakeAppImage.Cleanup())
		h.AssertNil(t, os.RemoveAll(tmpDir))
		mockCtrl.Finish()
	})

	when("#Export", func() {
		when("previous image exists", func() {
			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "previous-image-exists", "layers"), opts.LayersDir)

				fakeAppImage.AddPreviousLayer("launcher-digest", "")
				fakeAppImage.AddPreviousLayer("local-reusable-layer-digest", "")
				fakeAppImage.AddPreviousLayer("launch-layer-no-local-dir-digest", "")
				fakeAppImage.AddPreviousLayer("process-types-digest", "")
				h.AssertNil(t, json.Unmarshal([]byte(`
{
   "buildpacks": [
      {
         "key": "buildpack.id",
         "layers": {
            "launch-layer-no-local-dir": {
               "sha": "launch-layer-no-local-dir-digest",
               "data": {
                  "oldkey": "oldval"
               }
            }
         }
      },
      {
         "key": "other.buildpack.id",
         "layers": {
            "layer4": {
               "sha": "orig-layer4-sha",
               "data": {
                  "layer4key": "layer4val"
               }
            },
            "local-reusable-layer": {
               "sha": "local-reusable-layer-digest"
            }
         }
      }
   ],
   "launcher": {
      "sha": "launcher-digest"
   },
   "process-types": {
      "sha": "process-types-digest"
   }
}`), &opts.OrigMetadata))
			})

			when("there are slices", func() {
				it.Before(func() {
					opts.LayersDir = filepath.Join("testdata", "exporter", "app-slices", "layers")
					layerFactory.EXPECT().
						SliceLayers(
							opts.AppDir,
							[]layers.Slice{
								{Paths: []string{"static/**/*.txt", "static/**/*.svg"}},
								{Paths: []string{"static/misc/resources/**/*.csv", "static/misc/resources/**/*.tps"}},
							},
						).
						Return([]layers.Layer{
							{ID: "slice-1", Digest: "slice-1-digest"},
							{ID: "slice-2", Digest: "slice-2-digest"},
							{ID: "slice-3", Digest: "slice-3-digest"},
						}, nil)
					fakeAppImage.AddPreviousLayer("slice-1-digest", "")
					opts.OrigMetadata.App = append(opts.OrigMetadata.App, lifecycle.LayerMetadata{SHA: "slice-1-digest"})
				})

				it("reuses slice layer if the sha matches the sha in the archive metadata", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)
					h.AssertContains(t, fakeAppImage.ReusedLayers(), "slice-1-digest")
					assertLogEntry(t, logHandler, "Reusing 1/3 app layer(s)")
					assertLogEntry(t, logHandler, "Adding 2/3 app layer(s)")
				})
			})

			it("creates app layer on Run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "app")
				assertLogEntry(t, logHandler, "Adding 1/1 app layer(s)")
			})

			it("creates config layer on Run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "config")
				assertAddLayerLog(t, logHandler, "config")
			})

			it("reuses launcher layer if the sha matches the sha in the metadata", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)
				h.AssertContains(t, fakeAppImage.ReusedLayers(), "launcher-digest")
				assertReuseLayerLog(t, logHandler, "launcher")
			})

			it("reuses process-types layer if the sha matches the sha in the metadata", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)
				h.AssertContains(t, fakeAppImage.ReusedLayers(), "process-types-digest")
				assertReuseLayerLog(t, logHandler, "process-types")
			})

			it("reuses launch layers when only layer.toml is present", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				h.AssertContains(t, fakeAppImage.ReusedLayers(), "launch-layer-no-local-dir-digest")
				assertReuseLayerLog(t, logHandler, "buildpack.id:launch-layer-no-local-dir")
			})

			it("reuses cached launch layers if the local sha matches the sha in the metadata", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				h.AssertContains(t, fakeAppImage.ReusedLayers(), "local-reusable-layer-digest")
				assertReuseLayerLog(t, logHandler, "other.buildpack.id:local-reusable-layer")
			})

			it("adds new launch layers", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "buildpack.id:new-launch-layer")
				assertAddLayerLog(t, logHandler, "buildpack.id:new-launch-layer")
			})

			it("adds new launch layers from a second buildpack", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "other.buildpack.id:new-launch-layer")
				assertAddLayerLog(t, logHandler, "other.buildpack.id:new-launch-layer")
			})

			it("only adds expected layers", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				// expects 4 layers
				// 1. app layer
				// 2. config layer
				// 3-4. buildpack layers
				h.AssertEq(t, fakeAppImage.NumberOfAddedLayers(), 4)
			})

			it("only reuses expected layers", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				// expects 4 layers
				// 1. launcher layer
				// 2. process-types layer
				// 3-4. buildpack layers
				h.AssertEq(t, len(fakeAppImage.ReusedLayers()), 4)
			})

			it("saves lifecycle metadata with layer info", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta lifecycle.LayersMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				t.Log("adds run image metadata to label")
				h.AssertEq(t, meta.RunImage.TopLayer, "some-top-layer-sha")
				h.AssertEq(t, meta.RunImage.Reference, "run-image-reference")

				t.Log("adds layer shas to metadata label")
				h.AssertEq(t, meta.App[0].SHA, "app-digest")
				h.AssertEq(t, meta.Config.SHA, "config-digest")
				h.AssertEq(t, meta.Launcher.SHA, "launcher-digest")
				h.AssertEq(t, meta.ProcessTypes.SHA, "process-types-digest")
				h.AssertEq(t, meta.Buildpacks[0].ID, "buildpack.id")
				h.AssertEq(t, meta.Buildpacks[0].Version, "1.2.3")
				h.AssertEq(t, meta.Buildpacks[0].Layers["launch-layer-no-local-dir"].SHA, "launch-layer-no-local-dir-digest")
				h.AssertEq(t, meta.Buildpacks[0].Layers["new-launch-layer"].SHA, "new-launch-layer-digest")
				h.AssertEq(t, meta.Buildpacks[1].ID, "other.buildpack.id")
				h.AssertEq(t, meta.Buildpacks[1].Version, "4.5.6")
				h.AssertEq(t, meta.Buildpacks[1].Layers["new-launch-layer"].SHA, "new-launch-layer-digest")

				t.Log("adds buildpack layer metadata to label")
				h.AssertEq(t, meta.Buildpacks[0].Layers["launch-layer-no-local-dir"].Data, map[string]interface{}{
					"mykey": "updated launch-layer-no-local-dir val",
				})
				h.AssertEq(t, meta.Buildpacks[0].Layers["new-launch-layer"].Data, map[string]interface{}{
					"somekey": "someval",
				})
				h.AssertEq(t, meta.Buildpacks[1].Layers["local-reusable-layer"].Data, map[string]interface{}{
					"mykey": "updated locally reusable layer metadata val",
				})
			})

			it("saves run image metadata to the resulting image", func() {
				opts.Stack = lifecycle.StackMetadata{
					RunImage: lifecycle.StackRunImageMetadata{
						Image:   "some/run",
						Mirrors: []string{"registry.example.com/some/run", "other.example.com/some/run"},
					},
				}
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta lifecycle.LayersMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}
				h.AssertNil(t, err)
				h.AssertEq(t, meta.Stack.RunImage.Image, "some/run")
				h.AssertEq(t, meta.Stack.RunImage.Mirrors, []string{"registry.example.com/some/run", "other.example.com/some/run"})
			})

			when("metadata.toml is missing bom and has empty process list", func() {
				it.Before(func() {
					err := ioutil.WriteFile(filepath.Join(opts.LayersDir, "config", "metadata.toml"), []byte(`
processes = []

[[buildpacks]]
id = "buildpack.id"
version = "1.2.3"

[[buildpacks]]
id = "other.buildpack.id"
version = "4.5.6"
`),
						os.ModePerm,
					)
					h.AssertNil(t, err)
				})

				it("BOM is null and processes is an empty array in the label", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					metadataJSON, err := fakeAppImage.Label("io.buildpacks.build.metadata")
					h.AssertNil(t, err)

					expectedJSON := `
{
  "bom": null,
  "buildpacks": [
    {
      "id": "buildpack.id",
      "version": "1.2.3"
    },
    {
      "id": "other.buildpack.id",
      "version": "4.5.6"
    }
  ],
  "launcher": {
    "version": "1.2.3",
    "source": {
      "git": {
        "repository": "github.com/buildpacks/lifecycle",
        "commit": "asdf1234"
      }
    }
  },
  "processes": []
}
`
					h.AssertJSONEq(t, expectedJSON, metadataJSON)
				})
			})

			it("combines metadata.toml with launcher config to create build label", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.build.metadata")
				h.AssertNil(t, err)

				expectedJSON := `{
  "bom": [
    {
      "name": "Spring Auto-reconfiguration",
      "version": "2.7.0",
      "buildpack": {
        "id": "buildpack.id",
        "version": "1.2.3"
      },
      "metadata": {
        "licenses": [
          {
            "type": "Apache-2.0"
          }
        ],
        "sha256": "0d524877db7344ec34620f7e46254053568292f5ce514f74e3a0e9b2dbfc338b",
        "stacks": [
          "io.buildpacks.stacks.bionic",
          "org.cloudfoundry.stacks.cflinuxfs3"
        ],
        "uri": "https://example.com"
      }
    }
  ],
  "buildpacks": [
    {
      "id": "buildpack.id",
      "version": "1.2.3",
      "homepage": "buildpack homepage"
    },
    {
      "id": "other.buildpack.id",
      "version": "4.5.6",
      "homepage": "other buildpack homepage"
    }
  ],
  "launcher": {
    "version": "1.2.3",
    "source": {
      "git": {
        "repository": "github.com/buildpacks/lifecycle",
        "commit": "asdf1234"
      }
    }
  },
  "processes": [
    {
      "type": "some-process-type",
      "direct": true,
      "command": "/some/command",
      "args": ["some", "command", "args"],
      "buildpackID": "buildpack.id"
    }
  ]
}
`
				h.AssertJSONEq(t, expectedJSON, metadataJSON)
			})

			when("there is project metadata", func() {
				it("saves metadata with project info", func() {
					opts.Project = lifecycle.ProjectMetadata{
						Source: &lifecycle.ProjectSource{
							Type: "git",
							Version: map[string]interface{}{
								"commit": "abcd1234",
							},
							Metadata: map[string]interface{}{
								"repository": "github.com/buildpack/lifecycle",
								"branch":     "master",
							},
						}}
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					projectJSON, err := fakeAppImage.Label("io.buildpacks.project.metadata")
					h.AssertNil(t, err)

					var projectMD lifecycle.ProjectMetadata
					if err := json.Unmarshal([]byte(projectJSON), &projectMD); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}

					t.Log("adds project metadata to label")
					h.AssertEq(t, projectMD, opts.Project)
				})
			})

			it("sets CNB_LAYERS_DIR", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := fakeAppImage.Env("CNB_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, opts.LayersDir)
			})

			it("sets CNB_APP_DIR", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := opts.WorkingImage.Env("CNB_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, opts.AppDir)
			})

			it("sets CNB_PLATFORM_API", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := opts.WorkingImage.Env("CNB_PLATFORM_API")
				h.AssertNil(t, err)
				h.AssertEq(t, val, "0.4")
			})

			it("sets CNB_DEPRECATION_MODE=quiet", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := opts.WorkingImage.Env("CNB_DEPRECATION_MODE")
				h.AssertNil(t, err)
				h.AssertEq(t, val, "quiet")
			})

			when("PATH", func() {
				it.Before(func() {
					h.AssertNil(t, fakeAppImage.SetEnv("PATH", "some-path"))
				})

				when("platform API >= 0.4", func() {
					it.Before(func() {
						exporter.PlatformAPI = api.MustParse("0.4")
					})

					it("prepends the process and lifecycle dirs to PATH", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						val, err := opts.WorkingImage.Env("PATH")
						h.AssertNil(t, err)
						if runtime.GOOS == "windows" {
							h.AssertEq(t, val, `c:\cnb\process;c:\cnb\lifecycle;some-path`)
						} else {
							h.AssertEq(t, val, `/cnb/process:/cnb/lifecycle:some-path`)
						}
					})
				})

				when("platform API < 0.4", func() {
					it.Before(func() {
						exporter.PlatformAPI = api.MustParse("0.3")
					})

					it("doesn't prepend the process dir", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						val, err := opts.WorkingImage.Env("PATH")
						h.AssertNil(t, err)
						h.AssertEq(t, val, "some-path")
					})
				})
			})

			it("sets empty CMD", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := fakeAppImage.Cmd()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string(nil))
			})

			it("saves run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				h.AssertEq(t, fakeAppImage.IsSaved(), true)
			})

			when("image has a digest identifier", func() {
				var fakeRemoteDigest = "sha256:c27a27006b74a056bed5d9edcebc394783880abe8691a8c87c78b7cffa6fa5ad"

				it.Before(func() {
					digestRef, err := name.NewDigest("some-repo/app-image@" + fakeRemoteDigest)
					h.AssertNil(t, err)
					fakeAppImage.SetIdentifier(remote.DigestIdentifier{
						Digest: digestRef,
					})
				})

				it("outputs the digest", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					assertLogEntry(t, logHandler, `*** Digest: `+fakeRemoteDigest)
				})

				it("add the digest to the report", func() {
					report, err := exporter.Export(opts)
					h.AssertNil(t, err)

					h.AssertEq(t, report.Image.Digest, fakeRemoteDigest)
				})
			})

			when("image has an ID identifier", func() {
				it("outputs the imageID", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					assertLogEntry(t, logHandler, `*** Image ID: some-image-id`)
				})

				it("add the imageID to the report", func() {
					report, err := exporter.Export(opts)
					h.AssertNil(t, err)

					h.AssertEq(t, report.Image.ImageID, "some-image-id")
				})
			})

			it("outputs image names", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertLogEntry(t, logHandler, `*** Images (some-image-i):`)
				assertLogEntry(t, logHandler, fakeAppImage.Name())
				assertLogEntry(t, logHandler, opts.AdditionalNames[0])
				assertLogEntry(t, logHandler, opts.AdditionalNames[1])
			})

			when("one of the additional names fails", func() {
				it("outputs identifier and image name with error", func() {
					failingName := "not.a.tag@reference"
					opts.AdditionalNames = append(opts.AdditionalNames, failingName)

					_, err := exporter.Export(opts)
					h.AssertError(t, err, fmt.Sprintf("failed to write image to the following tags: [%s:", failingName))

					assertLogEntry(t, logHandler, `*** Images (some-image-i):`)
					assertLogEntry(t, logHandler, fakeAppImage.Name())
					assertLogEntry(t, logHandler, opts.AdditionalNames[0])
					assertLogEntry(t, logHandler, opts.AdditionalNames[1])
					assertLogEntry(t, logHandler, fmt.Sprintf("%s - could not parse reference", failingName))
				})
			})

			when("previous image metadata is missing buildpack for reused layer", func() {
				it.Before(func() {
					opts.OrigMetadata = lifecycle.LayersMetadata{
						Buildpacks: []lifecycle.BuildpackLayersMetadata{{}},
					}
				})

				it("returns an error", func() {
					_, err := exporter.Export(opts)
					h.AssertError(
						t,
						err,
						"cannot reuse 'buildpack.id:launch-layer-no-local-dir', previous image has no metadata for layer 'buildpack.id:launch-layer-no-local-dir'",
					)
				})
			})

			when("previous image metadata is missing reused layer", func() {
				it.Before(func() {
					opts.OrigMetadata = lifecycle.LayersMetadata{
						Buildpacks: []lifecycle.BuildpackLayersMetadata{{
							ID:     "buildpack.id",
							Layers: map[string]lifecycle.BuildpackLayerMetadata{},
						}},
					}
				})

				it("returns an error", func() {
					_, err := exporter.Export(opts)
					h.AssertError(
						t,
						err,
						"cannot reuse 'buildpack.id:launch-layer-no-local-dir', previous image has no metadata for layer 'buildpack.id:launch-layer-no-local-dir'",
					)
				})
			})

			it("saves the image for all provided AdditionalNames", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)
				h.AssertContains(t, fakeAppImage.SavedNames(), append(opts.AdditionalNames, fakeAppImage.Name())...)
			})

			it("adds all names to the report", func() {
				report, err := exporter.Export(opts)
				h.AssertNil(t, err)
				h.AssertContains(t, report.Image.Tags, append(opts.AdditionalNames, fakeAppImage.Name())...)
			})

			it("adds buildpack-provided labels to the image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)
				label, err := fakeAppImage.Label("some.label.key")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "some-label-value")
				label, err = fakeAppImage.Label("other.label.key")
				h.AssertNil(t, err)
				h.AssertEq(t, label, "other-label-value")
			})
		})

		when("previous image doesn't exist", func() {
			var (
				nonExistingOriginalImage *fakes.Image
			)

			it.Before(func() {
				var err error
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "previous-image-not-exist", "layers"), opts.LayersDir)
				opts.AppDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "previous-image-not-exist", "layers", "app"))
				h.AssertNil(t, err)

				// TODO : this is an hacky way to create a non-existing image and should be improved in imgutil
				nonExistingOriginalImage = fakes.NewImage("app/original-image", "", nil)
				h.AssertNil(t, nonExistingOriginalImage.Delete())
			})

			it.After(func() {
				h.AssertNil(t, nonExistingOriginalImage.Cleanup())
			})

			when("there are slices", func() {
				it.Before(func() {
					opts.LayersDir = filepath.Join("testdata", "exporter", "app-slices", "layers")
					layerFactory.EXPECT().SliceLayers(
						opts.AppDir,
						[]layers.Slice{
							{Paths: []string{"static/**/*.txt", "static/**/*.svg"}},
							{Paths: []string{"static/misc/resources/**/*.csv", "static/misc/resources/**/*.tps"}},
						},
					).Return([]layers.Layer{
						{ID: "slice-1", Digest: "slice-1-digest"},
						{ID: "slice-2", Digest: "slice-2-digest"},
						{ID: "slice-3", Digest: "slice-3-digest"},
					}, nil)
				})

				it("exports slice layers", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)
					assertLogEntry(t, logHandler, "Adding 3/3 app layer(s)")
				})
			})

			it("creates app layer on Run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "app")
				assertLogEntry(t, logHandler, "Adding 1/1 app layer(s)")
			})

			it("creates config layer on Run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "config")
				assertAddLayerLog(t, logHandler, "config")
			})

			it("creates a launcher layer", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "launcher")
				assertAddLayerLog(t, logHandler, "launcher")
			})

			when("platform API is greater than 0.4", func() {
				it("creates process-types layer", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					assertHasLayer(t, fakeAppImage, "process-types")
					assertAddLayerLog(t, logHandler, "process-types")
				})
			})

			when("platform API is less than 0.4", func() {
				it("doesn't create process-types layer", func() {
					exporter.PlatformAPI = api.MustParse("0.3")
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					assertDoesNotHaveLayer(t, fakeAppImage, "process-types")
				})
			})

			it("adds launch layers", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)
				assertHasLayer(t, fakeAppImage, "buildpack.id:layer1")
				assertAddLayerLog(t, logHandler, "buildpack.id:layer1")

				assertHasLayer(t, fakeAppImage, "buildpack.id:layer2")
				assertAddLayerLog(t, logHandler, "buildpack.id:layer2")
			})

			it("only creates expected layers", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				// expects 6 layers
				// 1. app layer
				// 2. launcher layer
				// 3. config layer
				// 4. process-types layer
				// 5-6. buildpack layers
				h.AssertEq(t, fakeAppImage.NumberOfAddedLayers(), 6)
			})

			it("saves metadata with layer info", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta lifecycle.LayersMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				t.Log("adds run image metadata to label")
				h.AssertEq(t, meta.RunImage.TopLayer, "some-top-layer-sha")
				h.AssertEq(t, meta.RunImage.Reference, "run-image-reference")

				t.Log("adds layer shas to metadata label")
				h.AssertEq(t, meta.App[0].SHA, "app-digest")
				h.AssertEq(t, meta.Config.SHA, "config-digest")
				h.AssertEq(t, meta.Launcher.SHA, "launcher-digest")
				h.AssertEq(t, meta.Buildpacks[0].ID, "buildpack.id")
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer1"].SHA, "layer1-digest")
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer2"].SHA, "layer2-digest")

				t.Log("adds buildpack layer metadata to label")
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer1"].Data, map[string]interface{}{
					"mykey": "new val",
				})

				t.Log("defaults to nil store")
				h.AssertNil(t, meta.Buildpacks[0].Store)
			})

			when("there are store.toml files", func() {
				it.Before(func() {
					path := filepath.Join(opts.LayersDir, "buildpack.id", "store.toml")
					h.AssertNil(t, ioutil.WriteFile(path, []byte("[metadata]\n  key = \"val\""), 0777))
				})

				it("saves store metadata", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
					h.AssertNil(t, err)

					var meta lifecycle.LayersMetadata
					if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}

					h.AssertEq(t, meta.Buildpacks[0].Store, &lifecycle.BuildpackStore{Data: map[string]interface{}{
						"key": "val",
					}})
				})
			})

			when("there is project metadata", func() {
				it("saves metadata with project info", func() {
					opts.Project = lifecycle.ProjectMetadata{
						Source: &lifecycle.ProjectSource{
							Type: "git",
							Version: map[string]interface{}{
								"commit": "abcd1234",
							},
							Metadata: map[string]interface{}{
								"repository": "github.com/buildpack/lifecycle",
								"branch":     "master",
							},
						}}
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					projectJSON, err := fakeAppImage.Label("io.buildpacks.project.metadata")
					h.AssertNil(t, err)

					var projectMD lifecycle.ProjectMetadata
					if err := json.Unmarshal([]byte(projectJSON), &projectMD); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}

					t.Log("adds project metadata to label")
					h.AssertEq(t, projectMD, opts.Project)
				})
			})

			it("sets CNB_LAYERS_DIR", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := fakeAppImage.Env("CNB_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, opts.LayersDir)
			})

			it("sets CNB_APP_DIR", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := fakeAppImage.Env("CNB_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, opts.AppDir)
			})

			when("default process type is set", func() {
				it.Before(func() {
					opts.DefaultProcessType = "some-process-type"
				})

				when("platform API is < 0.4", func() {
					it.Before(func() {
						exporter.PlatformAPI = api.MustParse("0.3")
					})

					it("sets CNB_PROCESS_TYPE", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						val, err := fakeAppImage.Env("CNB_PROCESS_TYPE")
						h.AssertNil(t, err)
						h.AssertEq(t, val, "some-process-type")
					})

					it("sets ENTRYPOINT to launcher", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						ep, err := fakeAppImage.Entrypoint()
						h.AssertNil(t, err)
						h.AssertEq(t, len(ep), 1)
						if runtime.GOOS == "windows" {
							h.AssertEq(t, ep[0], `c:\cnb\lifecycle\launcher.exe`)
						} else {
							h.AssertEq(t, ep[0], `/cnb/lifecycle/launcher`)
						}
					})

					when("default process type is not in metadata.toml", func() {
						it("returns an error", func() {
							opts.DefaultProcessType = "some-missing-process"
							_, err := exporter.Export(opts)
							h.AssertError(t, err, "default process type 'some-missing-process' not present in list [some-process-type]")
						})
					})
				})

				when("platform API is >= 0.4", func() {
					it.Before(func() {
						exporter.PlatformAPI = api.MustParse("0.4")
					})

					it("sets the ENTRYPOINT to the default process", func() {
						opts.DefaultProcessType = "some-process-type"
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						ep, err := fakeAppImage.Entrypoint()
						h.AssertNil(t, err)
						h.AssertEq(t, len(ep), 1)
						if runtime.GOOS == "windows" {
							h.AssertEq(t, ep[0], `c:\cnb\process\some-process-type.exe`)
						} else {
							h.AssertEq(t, ep[0], `/cnb/process/some-process-type`)
						}
					})

					it("does not set CNB_PROCESS_TYPE", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						val, err := fakeAppImage.Env("CNB_PROCESS_TYPE")
						h.AssertNil(t, err)
						h.AssertEq(t, val, "")
					})

					when("default process type is not in metadata.toml", func() {
						it("warns and sets the ENTRYPOINT to launcher", func() {
							opts.DefaultProcessType = "some-missing-process"
							_, err := exporter.Export(opts)
							h.AssertNil(t, err)

							assertLogEntry(t, logHandler, "default process type 'some-missing-process' not present in list [some-process-type]")
							ep, err := fakeAppImage.Entrypoint()
							h.AssertNil(t, err)
							h.AssertEq(t, len(ep), 1)
							if runtime.GOOS == "windows" {
								h.AssertEq(t, ep[0], `c:\cnb\lifecycle\launcher.exe`)
							} else {
								h.AssertEq(t, ep[0], `/cnb/lifecycle/launcher`)
							}
						})
					})
				})
			})

			when("default process type is empty", func() {
				when("platform API is >= 0.4", func() {
					it.Before(func() {
						exporter.PlatformAPI = api.MustParse("0.4")
					})

					when("there is exactly one process", func() {
						it("sets the ENTRYPOINT to the only process", func() {
							_, err := exporter.Export(opts)
							h.AssertNil(t, err)

							ep, err := fakeAppImage.Entrypoint()
							h.AssertNil(t, err)
							h.AssertEq(t, len(ep), 1)
							if runtime.GOOS == "windows" {
								h.AssertEq(t, ep[0], `c:\cnb\process\some-process-type.exe`)
							} else {
								h.AssertEq(t, ep[0], `/cnb/process/some-process-type`)
							}
						})
					})
				})

				when("platform API is < 0.4", func() {
					it.Before(func() {
						exporter.PlatformAPI = api.MustParse("0.3")
					})

					it("does not set CNB_PROCESS_TYPE", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						val, err := fakeAppImage.Env("CNB_PROCESS_TYPE")
						h.AssertNil(t, err)
						h.AssertEq(t, val, "")
					})

					it("sets ENTRYPOINT to launcher", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						ep, err := fakeAppImage.Entrypoint()
						h.AssertNil(t, err)
						h.AssertEq(t, len(ep), 1)
						if runtime.GOOS == "windows" {
							h.AssertEq(t, ep[0], `c:\cnb\lifecycle\launcher.exe`)
						} else {
							h.AssertEq(t, ep[0], `/cnb/lifecycle/launcher`)
						}
					})
				})
			})

			it("sets empty CMD", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := fakeAppImage.Cmd()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string(nil))
			})

			it("saves the image for all provided AdditionalNames", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)
				h.AssertContains(t, fakeAppImage.SavedNames(), append(opts.AdditionalNames, fakeAppImage.Name())...)
			})

			when("build.toml", func() {
				when("platform api >= 0.5", func() {
					it.Before(func() {
						exporter.PlatformAPI = api.MustParse("0.5")
						exporter.Buildpacks = []lifecycle.Buildpack{
							{ID: "buildpack.id", Version: "1.2.3", API: "0.5"},
							{ID: "other.buildpack.id", Version: "4.5.6", Optional: false, API: "0.5"},
						}
						opts.LayersDir = filepath.Join("testdata", "exporter", "build-metadata", "layers")
					})

					it("adds build bom entries to the report", func() {
						report, err := exporter.Export(opts)
						h.AssertNil(t, err)

						h.AssertEq(t, report.Build.BOM, []lifecycle.BOMEntry{
							{
								Require: lifecycle.Require{
									Name:     "dep1",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: lifecycle.Buildpack{ID: "buildpack.id", Version: "1.2.3"},
							},
							{
								Require: lifecycle.Require{
									Name:     "dep2",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: lifecycle.Buildpack{ID: "other.buildpack.id", Version: "4.5.6"},
							},
						})
					})
				})
			})
		})

		when("buildpack requires an escaped id", func() {
			it.Before(func() {
				exporter.Buildpacks = []lifecycle.Buildpack{{ID: "some/escaped/bp/id"}}

				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "escaped-bpid", "layers"), opts.LayersDir)
			})

			it("exports layers from the escaped id path", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "some/escaped/bp/id:some-layer")
				assertAddLayerLog(t, logHandler, "some/escaped/bp/id:some-layer")
			})

			it("exports buildpack metadata with unescaped id", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta lifecycle.LayersMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				h.AssertEq(t, meta.Buildpacks[0].ID, "some/escaped/bp/id")
				h.AssertEq(t, len(meta.Buildpacks[0].Layers), 1)
			})
		})

		when("there is an invalid layer.toml", func() {
			var (
				nonExistingOriginalImage *fakes.Image
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "bad-layer", "layers"), opts.LayersDir)

				var err error
				opts.AppDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "bad-layer", "layers", "app"))
				h.AssertNil(t, err)

				// TODO : this is an hacky way to create a non-existing image and should be improved in imgutil
				nonExistingOriginalImage = fakes.NewImage("app/original-image", "", nil)
				h.AssertNil(t, nonExistingOriginalImage.Delete())
			})

			it.After(func() {
				h.AssertNil(t, nonExistingOriginalImage.Cleanup())
			})

			it("returns an error", func() {
				_, err := exporter.Export(opts)
				h.AssertError(
					t,
					err,
					"failed to parse metadata for layers '[buildpack.id:bad-layer]'",
				)
			})
		})

		when("there is a launch=true cache=true layer without contents", func() {
			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "cache-layer-no-contents", "layers"), opts.LayersDir)
				var err error
				opts.AppDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "cache-layer-no-contents", "layers", "app"))
				h.AssertNil(t, err)
			})

			it("returns an error", func() {
				_, err := exporter.Export(opts)
				h.AssertError(
					t,
					err,
					"layer 'buildpack.id:cache-layer-no-contents' is cache=true but has no contents",
				)
			})
		})
	})
}

func createTestLayer(id string, tmpDir string) (layers.Layer, error) {
	tarPath := filepath.Join(tmpDir, "artifacts", strings.Replace(id, "/", "_", -1))
	f, err := os.Create(tarPath)
	if err != nil {
		return layers.Layer{}, err
	}
	defer f.Close()
	_, err = f.Write([]byte(testLayerContents(id)))
	if err != nil {
		return layers.Layer{}, err
	}
	return layers.Layer{
		ID:      id,
		TarPath: tarPath,
		Digest:  testLayerDigest(id),
	}, nil
}

func assertHasLayer(t *testing.T, fakeAppImage *fakes.Image, id string) {
	t.Helper()

	rc, err := fakeAppImage.GetLayer(testLayerDigest(id))
	h.AssertNil(t, err)
	defer rc.Close()
	contents, err := ioutil.ReadAll(rc)
	h.AssertNil(t, err)
	h.AssertEq(t, string(contents), testLayerContents(id))
}

func assertDoesNotHaveLayer(t *testing.T, fakeAppImage *fakes.Image, id string) {
	t.Helper()

	_, err := fakeAppImage.GetLayer(testLayerDigest(id))
	h.AssertNotNil(t, err)
}

func assertAddLayerLog(t *testing.T, logHandler *memory.Handler, id string) {
	t.Helper()
	assertLogEntry(t, logHandler, fmt.Sprintf("Adding layer '%s'", id))
	assertLogEntry(t, logHandler, fmt.Sprintf("Layer '%s' SHA: %s", id, testLayerDigest(id)))
}

func assertReuseLayerLog(t *testing.T, logHandler *memory.Handler, id string) {
	t.Helper()
	assertLogEntry(t, logHandler, fmt.Sprintf("Reusing layer '%s'", id))
	assertLogEntry(t, logHandler, fmt.Sprintf("Layer '%s' SHA: %s", id, testLayerDigest(id)))
}

func testLayerDigest(id string) string {
	parts := strings.Split(id, ":")
	return parts[len(parts)-1] + "-digest"
}

func testLayerContents(id string) string {
	parts := strings.Split(id, ":")
	return parts[len(parts)-1] + "-contents"
}

func assertLogEntry(t *testing.T, logHandler *memory.Handler, expected string) {
	t.Helper()
	var messages []string
	for _, le := range logHandler.Entries {
		messages = append(messages, le.Message)
		if strings.Contains(le.Message, expected) {
			return
		}
	}
	t.Fatalf("Expected log entries %+v to contain %s", messages, expected)
}
