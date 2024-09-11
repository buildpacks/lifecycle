package phase_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	specreport "github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/path"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/phase/testmock"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExporter(t *testing.T) {
	spec.Run(t, "Exporter", testExporter, spec.Parallel(), spec.Report(specreport.Terminal{}))
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	var (
		exporter     *phase.Exporter
		tmpDir       string
		mockCtrl     *gomock.Controller
		layerFactory *testmock.MockLayerFactory
		fakeAppImage *fakes.Image
		logHandler   = memory.New()
		opts         = phase.ExportOptions{
			RunImageRef:     "run-image-reference",
			AdditionalNames: []string{},
		}
		platformAPI = api.Platform.Latest()
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		layerFactory = testmock.NewMockLayerFactory(mockCtrl)

		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle.exporter.layer")
		h.AssertNil(t, err)

		launcherPath, err := filepath.Abs(filepath.Join("testdata", "exporter", "launcher"))
		h.AssertNil(t, err)

		opts.LauncherConfig = phase.LauncherConfig{
			Path: launcherPath,
			Metadata: files.LauncherMetadata{
				Version: "1.2.3",
				Source: files.SourceMetadata{
					Git: files.GitMetadata{
						Repository: "github.com/buildpacks/lifecycle",
						Commit:     "asdf1234",
					},
				},
			},
		}

		opts.LayersDir = filepath.Join(tmpDir, "layers")
		h.AssertNil(t, os.Mkdir(opts.LayersDir, 0777))
		h.AssertNil(t, os.WriteFile(filepath.Join(tmpDir, "launcher"), []byte("some-launcher"), 0600))
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
			DirLayer(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(id string, dir string, createdBy string) (layers.Layer, error) {
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
						Type: "some-process-type",
						Command: launch.NewRawCommand([]string{"/some/command"}).
							WithPlatformAPI(platformAPI),
						Args:        []string{"some", "command", "args"},
						Direct:      true,
						BuildpackID: "buildpack.id",
						PlatformAPI: platformAPI,
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

		exporter = &phase.Exporter{
			Buildpacks: []buildpack.GroupElement{
				{ID: "buildpack.id", Version: "1.2.3", API: api.Buildpack.Latest().String()},
				{ID: "other.buildpack.id", Version: "4.5.6", API: api.Buildpack.Latest().String(), Optional: false},
			},
			LayerFactory: layerFactory,
			Logger:       &log.Logger{Handler: logHandler},
			PlatformAPI:  platformAPI,
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
				fakeAppImage.AddPreviousLayer("launch.sbom-digest", "")
				h.AssertNil(t, json.Unmarshal([]byte(`
{
   "sbom": {
     "sha": "launch.sbom-digest"
   },
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
					opts.OrigMetadata.App = append(opts.OrigMetadata.App, files.LayerMetadata{SHA: "slice-1-digest"})
				})

				it("reuses slice layer if the sha matches the sha in the archive metadata", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)
					h.AssertContains(t, fakeAppImage.ReusedLayers(), "slice-1-digest")
					assertLogEntry(t, logHandler, "Reused 1/3 app layer(s)")
					assertLogEntry(t, logHandler, "Added 2/3 app layer(s)")
				})
			})

			when("structured SBOM", func() {
				when("there is a 'launch=true' layer with a bom.<ext> file", func() {
					it("reuses bom layers if the sha matches the sha in the metadata", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)
						h.AssertContains(t, fakeAppImage.ReusedLayers(), "launch.sbom-digest")
						assertReuseLayerLog(t, logHandler, "buildpacksio/lifecycle:launch.sbom")
					})
				})
			})

			it("creates app layer on run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "app")
				assertLogEntry(t, logHandler, "Added 1/1 app layer(s)")
			})

			it("creates config layer on Run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "config")
				assertAddLayerLog(t, logHandler, "buildpacksio/lifecycle:config")
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

			when("the launch flag is in the top level table", func() {
				it.Before(func() {
					exporter.Buildpacks = []buildpack.GroupElement{{ID: "bad.buildpack.id", API: api.Buildpack.Latest().String()}}
					fakeAppImage.AddPreviousLayer("bad-layer", "")
					opts.OrigMetadata = files.LayersMetadata{
						Buildpacks: []buildpack.LayersMetadata{{
							ID:     "bad.buildpack.id",
							Layers: map[string]buildpack.LayerMetadata{"bad-layer": {SHA: "bad-layer"}},
						}},
					}
				})

				it("should error", func() {
					_, err := exporter.Export(opts)
					h.AssertNotNil(t, err)
					expected := "failed to parse metadata for layers '[bad.buildpack.id:bad-layer]'"
					h.AssertStringContains(t, err.Error(), expected)
					h.AssertEq(t, len(fakeAppImage.ReusedLayers()), 0)
				})
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
				// 5. BOM layer
				h.AssertEq(t, len(fakeAppImage.ReusedLayers()), 5)
			})

			it("saves lifecycle metadata with layer info", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta files.LayersMetadata
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

			when("run image metadata", func() {
				it("saves run image metadata to the runImage key", func() {
					opts.RunImageForExport = files.RunImageForExport{
						Image:   "some/run",
						Mirrors: []string{"registry.example.com/some/run", "other.example.com/some/run"},
					}
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
					h.AssertNil(t, err)

					var meta files.LayersMetadata
					if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}
					h.AssertNil(t, err)
					h.AssertEq(t, meta.RunImage.Image, "some/run")
					h.AssertEq(t, meta.RunImage.Mirrors, []string{"registry.example.com/some/run", "other.example.com/some/run"})
					h.AssertEq(t, meta.Stack.RunImage.Image, meta.RunImage.Image)
					h.AssertEq(t, meta.Stack.RunImage.Mirrors, meta.RunImage.Mirrors)
				})
				when("platform api < 0.12", func() {
					platformAPI = api.MustParse("0.11")
					it("doesnt add new keys to the json", func() {
						opts.RunImageForExport = files.RunImageForExport{
							Image:   "some/run",
							Mirrors: []string{"registry.example.com/some/run", "other.example.com/some/run"},
						}
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertNil(t, err)

						var meta files.LayersMetadata
						if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
							t.Fatalf("badly formatted metadata: %s", err)
						}
						h.AssertNil(t, err)
						h.AssertEq(t, meta.Stack.RunImage.Image, "some/run")
						h.AssertEq(t, meta.Stack.RunImage.Mirrors, []string{"registry.example.com/some/run", "other.example.com/some/run"})
						h.AssertEq(t, meta.RunImage.Image, "")
						h.AssertEq(t, meta.RunImage.Mirrors, []string(nil))
					})
				})
			})
			when("platform api >= 0.12", func() {
				platformAPI = api.MustParse("0.12")
				it("saves run image metadata to the stack key even if it's a target", func() {
					opts.RunImageForExport = files.RunImageForExport{
						Image: "some/run",
					}
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
					h.AssertNil(t, err)

					var meta files.LayersMetadata
					if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}
					h.AssertNil(t, err)
					h.AssertEq(t, meta.RunImage.Image, "some/run")
					h.AssertNotNil(t, meta.Stack)
					h.AssertEq(t, meta.Stack.RunImage.Image, "some/run")
				})
			})

			when("build metadata label", func() {
				it("contains the expected information", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					metadataJSON, err := fakeAppImage.Label("io.buildpacks.build.metadata")
					h.AssertNil(t, err)

					t.Log("bom is omitted")
					t.Log("command is array")
					expectedJSON := `{
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
      "command": ["/some/command"],
      "args": ["some", "command", "args"],
      "buildpackID": "buildpack.id"
    }
  ]
}
`
					h.AssertJSONEq(t, expectedJSON, metadataJSON)
				})

				when("platform api < 0.9", func() {
					platformAPI = api.MustParse("0.8")

					when("metadata.toml is missing bom and has empty process list", func() {
						it.Before(func() {
							err := os.WriteFile(filepath.Join(opts.LayersDir, "config", "metadata.toml"), []byte(`
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

						it("bom is null and processes is an empty array", func() {
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

					it("contains the expected information", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						metadataJSON, err := fakeAppImage.Label("io.buildpacks.build.metadata")
						h.AssertNil(t, err)

						t.Log("command is string")
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
				})
			})

			when("there is project metadata", func() {
				it("saves metadata with project info", func() {
					opts.Project = files.ProjectMetadata{
						Source: &files.ProjectSource{
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

					var projectMD files.ProjectMetadata
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
				h.AssertEq(t, val, api.Platform.Latest().String())
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

			it("sets WorkingDir", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				val, err := fakeAppImage.WorkingDir()
				h.AssertNil(t, err)
				h.AssertEq(t, val, opts.AppDir)
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

			it("outputs saving message", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertLogEntry(t, logHandler, fmt.Sprintf("Saving %s...", fakeAppImage.Name()))
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
					opts.OrigMetadata = files.LayersMetadata{
						Buildpacks: []buildpack.LayersMetadata{{}},
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
					opts.OrigMetadata = files.LayersMetadata{
						Buildpacks: []buildpack.LayersMetadata{{
							ID:     "buildpack.id",
							Layers: map[string]buildpack.LayerMetadata{},
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
					assertLogEntry(t, logHandler, "Added 3/3 app layer(s)")
				})
			})

			when("structured SBOM", func() {
				when("there is a 'launch=true' layer with a bom.<ext> file", func() {
					it("creates a bom layer on Run image", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						assertHasLayer(t, fakeAppImage, "launch.sbom")
						assertAddLayerLog(t, logHandler, "buildpacksio/lifecycle:launch.sbom")

						var result struct {
							BOM struct {
								SHA string `json:"sha"`
							} `json:"sbom"`
						}

						data, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
						h.AssertNil(t, err)

						h.AssertNil(t, json.Unmarshal([]byte(data), &result))
						h.AssertEq(t, result.BOM.SHA, "launch.sbom-digest")
					})
				})
			})

			it("creates app layer on run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "app")
				assertLogEntry(t, logHandler, "Added 1/1 app layer(s)")
			})

			it("creates config layer on Run image", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "config")
				assertAddLayerLog(t, logHandler, "buildpacksio/lifecycle:config")
			})

			it("creates a launcher layer", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "launcher")
				assertAddLayerLog(t, logHandler, "launcher")
			})

			it("creates process-types layer", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				assertHasLayer(t, fakeAppImage, "process-types")
				assertAddLayerLog(t, logHandler, "process-types")
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
				// 7. BOM layer
				h.AssertEq(t, fakeAppImage.NumberOfAddedLayers(), 7)
			})

			it("saves metadata with layer info", func() {
				_, err := exporter.Export(opts)
				h.AssertNil(t, err)

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta files.LayersMetadata
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
					h.AssertNil(t, os.WriteFile(path, []byte("[metadata]\n  key = \"val\""), 0600))
				})

				it("saves store metadata", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
					h.AssertNil(t, err)

					var meta files.LayersMetadata
					if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}

					h.AssertEq(t, meta.Buildpacks[0].Store, &buildpack.StoreTOML{Data: map[string]interface{}{
						"key": "val",
					}})
				})
			})

			when("there is project metadata", func() {
				it("saves metadata with project info", func() {
					opts.Project = files.ProjectMetadata{
						Source: &files.ProjectSource{
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

					var projectMD files.ProjectMetadata
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
		})

		when("default process", func() {
			when("-process-type is set", func() {
				when("it is set to an existing type", func() {
					it.Before(func() {
						opts.DefaultProcessType = "some-process-type"
						h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "default-process", "metadata-with-no-default", "layers"), opts.LayersDir)
					})

					it("sets the ENTRYPOINT to this process type", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						assertHasEntrypoint(t, fakeAppImage, filepath.Join(path.RootDir, "cnb", "process", "some-process-type"+path.ExecExt))
					})

					it("doesn't set CNB_PROCESS_TYPE", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						val, err := fakeAppImage.Env("CNB_PROCESS_TYPE")
						h.AssertNil(t, err)
						h.AssertEq(t, val, "")
					})
				})

				when("it is set to a process type that doesn't exist", func() {
					it.Before(func() {
						opts.DefaultProcessType = "some-non-existing-process-type"
						h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "default-process", "metadata-with-no-default", "layers"), opts.LayersDir)
					})
					it("fails", func() {
						_, err := exporter.Export(opts)
						h.AssertError(t, err, "tried to set some-non-existing-process-type to default but it doesn't exist")
					})
				})
			})

			when("-process-type is not set", func() {
				when("buildpack-default-process-type is not set in metadata.toml", func() {
					it.Before(func() {
						h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "default-process", "metadata-with-no-default", "layers"), opts.LayersDir)
					})

					it("send an info message that there is no default process, and sets the ENTRYPOINT to the launcher", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)
						assertLogEntry(t, logHandler, "no default process type")
						assertHasEntrypoint(t, fakeAppImage, filepath.Join(path.RootDir, "cnb", "lifecycle", "launcher"+path.ExecExt))
					})
				})

				when("buildpack-default-process-type is set in metadata.toml", func() {
					it.Before(func() {
						h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "default-process", "metadata-with-default", "layers"), opts.LayersDir)
						layerFactory.EXPECT().
							ProcessTypesLayer(launch.Metadata{
								Processes: []launch.Process{
									{
										Type: "some-process-type",
										Command: launch.NewRawCommand([]string{"/some/command"}).
											WithPlatformAPI(api.Platform.Latest()),
										Args:        []string{"some", "command", "args"},
										Direct:      true,
										BuildpackID: "buildpack.id",
									}},
							}).
							DoAndReturn(func(_ launch.Metadata) (layers.Layer, error) {
								return createTestLayer("process-types", tmpDir)
							}).
							AnyTimes()
					})

					it("sets the ENTRYPOINT to this process type", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)
						assertHasEntrypoint(t, fakeAppImage, filepath.Join(path.RootDir, "cnb", "process", "some-process-type"+path.ExecExt))
					})

					it("doesn't set CNB_PROCESS_TYPE", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						val, err := fakeAppImage.Env("CNB_PROCESS_TYPE")
						h.AssertNil(t, err)
						h.AssertEq(t, val, "")
					})
				})
			})
		})

		when("report.toml", func() {
			when("manifest size", func() {
				var fakeRemoteManifestSize int64
				it.Before(func() {
					opts.LayersDir = filepath.Join("testdata", "exporter", "empty-metadata", "layers")
				})

				when("image has a manifest", func() {
					it.Before(func() {
						fakeRemoteManifestSize = 12345
						fakeAppImage.SetManifestSize(fakeRemoteManifestSize)
					})

					it("outputs the manifest size", func() {
						_, err := exporter.Export(opts)
						h.AssertNil(t, err)

						assertLogEntry(t, logHandler, fmt.Sprintf("*** Manifest Size: %d", fakeRemoteManifestSize))
					})

					it("add the manifest size to the report", func() {
						report, err := exporter.Export(opts)
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
						report, err := exporter.Export(opts)
						h.AssertNil(t, err)

						h.AssertEq(t, report.Image.ManifestSize, int64(0))
					})
				})
			})

			when("image has a digest identifier", func() {
				var fakeRemoteDigest = "sha256:c27a27006b74a056bed5d9edcebc394783880abe8691a8c87c78b7cffa6fa5ad"

				it.Before(func() {
					opts.LayersDir = filepath.Join("testdata", "exporter", "empty-metadata", "layers")
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
				it.Before(func() {
					opts.LayersDir = filepath.Join("testdata", "exporter", "empty-metadata", "layers")
				})

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

			when("build bom", func() {
				it.Before(func() {
					opts.LayersDir = filepath.Join("testdata", "exporter", "build-metadata", "layers")
				})

				when("platform api >= 0.9", func() {
					it("does not add build bom entries to the report", func() {
						report, err := exporter.Export(opts)
						h.AssertNil(t, err)

						var empty []buildpack.BOMEntry
						h.AssertEq(t, report.Build.BOM, empty)
					})
				})

				when("platform api 0.5 to 0.8", func() {
					it.Before(func() {
						exporter.PlatformAPI = api.MustParse("0.8")
					})

					when("valid", func() {
						it("adds build bom entries to the report", func() {
							report, err := exporter.Export(opts)
							h.AssertNil(t, err)

							h.AssertEq(t, report.Build.BOM, []buildpack.BOMEntry{
								{
									Require: buildpack.Require{
										Name:     "dep1",
										Metadata: map[string]interface{}{"version": string("v1")},
									},
									Buildpack: buildpack.GroupElement{ID: "buildpack.id", Version: "1.2.3"},
								},
								{
									Require: buildpack.Require{
										Name:     "dep2",
										Metadata: map[string]interface{}{"version": string("v1")},
									},
									Buildpack: buildpack.GroupElement{ID: "other.buildpack.id", Version: "4.5.6"},
								},
							})
						})
					})

					when("invalid", func() {
						it.Before(func() {
							opts.LayersDir = filepath.Join("testdata", "exporter", "build-metadata", "bad-layers")
						})

						it("returns an error", func() {
							_, err := exporter.Export(opts)
							h.AssertError(t, err, "toml")
						})
					})
				})
			})
		})

		when("buildpack requires an escaped id", func() {
			it.Before(func() {
				exporter.Buildpacks = []buildpack.GroupElement{{ID: "some/escaped/bp/id", API: api.Buildpack.Latest().String()}}

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

				var meta files.LayersMetadata
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

		when("buildpacksio SBOM", func() {
			it.Before(func() {
				exporter.PlatformAPI = api.MustParse("0.11")
			})

			when("missing from default directory", func() {
				it.Before(func() {
					h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "empty-metadata", "layers"), opts.LayersDir) // don't care
				})

				it("should display that the SBOM is missing", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					extensions := phase.SBOMExtensions()
					for _, component := range []string{"lifecycle", "launcher"} {
						for _, extension := range extensions {
							assertLogEntry(t, logHandler, fmt.Sprintf("Did not find SBOM %s.%s", component, extension))
						}
					}
				})
			})

			when("custom launcher SBOM is provided", func() {
				it.Before(func() {
					h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "launcher-sbom", "layers"), opts.LayersDir)
					// later we'll assert that this file doesn't get copied, but that's only meaningful if it exists at the source
					h.AssertPathExists(t, filepath.Join(opts.LayersDir, "some-launcher-sbom-dir", "a-regular-file.txt"))
					opts.LauncherConfig.SBOMDir = filepath.Join(opts.LayersDir, "some-launcher-sbom-dir")
				})

				it("copies sboms from the launcher SBOM directory", func() {
					_, err := exporter.Export(opts)
					h.AssertNil(t, err)

					h.AssertPathExists(t, filepath.Join(opts.LayersDir, "sbom", "launch", "buildpacksio_lifecycle", "launcher", "some-sbom-file.sbom.spdx.json"))
					h.AssertPathDoesNotExist(t, filepath.Join(opts.LayersDir, "sbom", "launch", "buildpacksio_lifecycle", "launcher", "a-regular-file.txt"))
				})
			})
		})
	})
}

func assertHasEntrypoint(t *testing.T, image *fakes.Image, entrypointPath string) {
	ep, err := image.Entrypoint()
	h.AssertNil(t, err)
	h.AssertEq(t, len(ep), 1)
	h.AssertEq(t, ep[0], entrypointPath)
}

func createTestLayer(id string, tmpDir string) (layers.Layer, error) {
	tarPath := filepath.Join(tmpDir, "artifacts", strings.ReplaceAll(id, "/", "_"))
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
	contents, err := io.ReadAll(rc)
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
