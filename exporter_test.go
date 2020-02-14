package lifecycle_test

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/apex/log/handlers/memory"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cache"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExporter(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Exporter", testExporter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	var (
		exporter        *lifecycle.Exporter
		fakeAppImage    *fakes.Image
		layersDir       string
		tmpDir          string
		appDir          string
		launcherConfig  lifecycle.LauncherConfig
		uid             = 1234
		gid             = 4321
		stack           = lifecycle.StackMetadata{}
		project         = lifecycle.ProjectMetadata{}
		additionalNames []string
		runImageRef     = "run-image-reference"
		logHandler      = memory.New()
	)

	it.Before(func() {
		tmpDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
		h.AssertNil(t, err)

		launcherPath, err := filepath.Abs(filepath.Join("testdata", "exporter", "launcher"))
		h.AssertNil(t, err)

		launcherConfig = lifecycle.LauncherConfig{
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

		layersDir = filepath.Join(tmpDir, "layers")
		h.AssertNil(t, os.Mkdir(layersDir, 0777))
		h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "launcher"), []byte("some-launcher"), 0777))

		fakeAppImage = fakes.NewImage(
			"some-repo/app-image",
			"some-top-layer-sha",
			local.IDIdentifier{
				ImageID: "some-image-id",
			},
		)

		additionalNames = []string{"some-repo/app-image:foo", "some-repo/app-image:bar"}

		exporter = &lifecycle.Exporter{
			ArtifactsDir: tmpDir,
			Buildpacks: []lifecycle.Buildpack{
				{ID: "buildpack.id", Version: "1.2.3"},
				{ID: "other.buildpack.id", Version: "4.5.6", Optional: false},
			},
			Logger: &log.Logger{Handler: logHandler},
			UID:    uid,
			GID:    gid,
		}
	})

	it.After(func() {
		fakeAppImage.Cleanup()
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#Export", func() {
		when("previous slice image exists", func() {
			var (
				fakeOriginalImage *fakes.Image
				fakeImageMetadata lifecycle.LayersMetadata
				sliceSHA          string
			)

			it.Before(func() {
				var err error

				// LayersDir is a tmp directory
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "app-slices", "layers"), layersDir)

				appDir, err = filepath.Abs(filepath.Join(layersDir, "app"))
				h.AssertNil(t, err)

				sliceConfigFilePath := filepath.Join(appDir, "static", "assets", "config.txt")
				sliceSvgFilePath := filepath.Join(appDir, "static", "assets", "logo.svg")

				localReusableLayerPath := filepath.Join(layersDir, "other.buildpack.id/local-reusable-layer")
				localReusableLayerSha := h.ComputeSHA256ForPath(t, localReusableLayerPath, uid, gid)
				launcherSHA := h.ComputeSHA256ForPath(t, launcherConfig.Path, uid, gid)
				sliceSHA = h.ComputeSHA256ForFiles(t, filepath.Join(layersDir, "slice-test.tar"), uid, gid, sliceConfigFilePath, sliceSvgFilePath)

				fakeOriginalImage = fakes.NewImage("app/original-image", "original-top-layer-sha",
					local.IDIdentifier{ImageID: "some-original-run-image-digest"},
				)
				fakeAppImage.AddPreviousLayer("sha256:"+localReusableLayerSha, "")
				fakeAppImage.AddPreviousLayer("sha256:"+launcherSHA, "")
				fakeAppImage.AddPreviousLayer("sha256:orig-launch-layer-no-local-dir-sha", "")
				fakeAppImage.AddPreviousLayer("sha256:"+sliceSHA, "")

				h.AssertNil(t, fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata",
					fmt.Sprintf(`
{
   "app": [
      {
         "sha": "sha256:%s"
      }
   ],
   "buildpacks": [
      {
         "key": "buildpack.id",
         "layers": {
            "launch-layer-no-local-dir": {
               "sha": "sha256:orig-launch-layer-no-local-dir-sha",
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
               "sha": "sha256:%s"
            }
         }
      }
   ],
   "launcher": {
      "sha": "sha256:%s"
   }
}`, sliceSHA, localReusableLayerSha, launcherSHA)))

				h.AssertNil(t, lifecycle.DecodeLabel(fakeOriginalImage, lifecycle.LayerMetadataLabel, &fakeImageMetadata))
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("reuses slice layer if the sha matches the sha in the archive metadata", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				sliceLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, "static", "misc", "resources", "reports", "report.tps"))
				h.AssertNil(t, err)

				assertTarFileExists(t, sliceLayerPath, filepath.Join(appDir, "static", "misc", "resources", "reports", "numbers.csv"), true)
				assertTarFileExists(t, sliceLayerPath, filepath.Join(appDir, "static", "misc", "resources", "reports", "report.tps"), true)

				appLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, ".hidden.txt"))
				h.AssertNil(t, err)

				assertTarFileExists(t, appLayerPath, filepath.Join(appDir, "static", "misc", "resources"), false)

				h.AssertContains(t, fakeAppImage.ReusedLayers(), "sha256:"+sliceSHA)
				assertLogEntry(t, logHandler, "Reusing 1/4 app layer(s)")
				assertLogEntry(t, logHandler, "Adding 3/4 app layer(s)")
			})
		})

		when("previous image exists", func() {
			var (
				fakeOriginalImage *fakes.Image
				fakeImageMetadata lifecycle.LayersMetadata
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "previous-image-exists", "layers"), layersDir)

				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "previous-image-exists", "layers", "app"))
				h.AssertNil(t, err)

				localReusableLayerPath := filepath.Join(layersDir, "other.buildpack.id/local-reusable-layer")
				localReusableLayerSha := h.ComputeSHA256ForPath(t, localReusableLayerPath, uid, gid)
				launcherSHA := h.ComputeSHA256ForPath(t, launcherConfig.Path, uid, gid)

				fakeOriginalImage = fakes.NewImage("app/original-image", "original-top-layer-sha",
					local.IDIdentifier{ImageID: "some-original-run-image-digest"},
				)
				fakeAppImage.AddPreviousLayer("sha256:"+localReusableLayerSha, "")
				fakeAppImage.AddPreviousLayer("sha256:"+launcherSHA, "")
				fakeAppImage.AddPreviousLayer("sha256:orig-launch-layer-no-local-dir-sha", "")

				h.AssertNil(t, fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata",
					fmt.Sprintf(`
{
   "buildpacks": [
      {
         "key": "buildpack.id",
         "layers": {
            "launch-layer-no-local-dir": {
               "sha": "sha256:orig-launch-layer-no-local-dir-sha",
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
               "sha": "sha256:%s"
            }
         }
      }
   ],
   "launcher": {
      "sha": "sha256:%s"
   }
}`, localReusableLayerSha, launcherSHA)))

				h.AssertNil(t, lifecycle.DecodeLabel(fakeOriginalImage, lifecycle.LayerMetadataLabel, &fakeImageMetadata))
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("creates app layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				appLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, ".hidden.txt"))
				h.AssertNil(t, err)

				assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
				assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
				assertLogEntry(t, logHandler, "Adding 1/1 app layer(s)")
			})

			it("creates config layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				configLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "config", "metadata.toml"))
				h.AssertNil(t, err)

				assertTarFileContents(t,
					configLayerPath,
					filepath.Join(layersDir, "config", "metadata.toml"),
					"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
				)
				assertTarFileOwner(t, configLayerPath, filepath.Join(layersDir, "config"), uid, gid)
				assertAddLayerLog(t, logHandler, "config", configLayerPath)
			})

			it("reuses launcher layer if the sha matches the sha in the metadata", func() {
				launcherLayerSHA := h.ComputeSHA256ForPath(t, launcherConfig.Path, uid, gid)
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))
				h.AssertContains(t, fakeAppImage.ReusedLayers(), "sha256:"+launcherLayerSHA)
				assertReuseLayerLog(t, logHandler, "launcher", launcherLayerSHA)
			})

			it("reuses launch layers when only layer.toml is present", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				h.AssertContains(t, fakeAppImage.ReusedLayers(), "sha256:orig-launch-layer-no-local-dir-sha")
				assertReuseLayerLog(t, logHandler, "buildpack.id:launch-layer-no-local-dir", "orig-launch-layer-no-local-dir-sha")
			})

			it("reuses cached launch layers if the local sha matches the sha in the metadata", func() {
				layer5sha := h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/local-reusable-layer"), uid, gid)

				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				h.AssertContains(t, fakeAppImage.ReusedLayers(), "sha256:"+layer5sha)
				assertReuseLayerLog(t, logHandler, "other.buildpack.id:local-reusable-layer", layer5sha)
			})

			it("adds new launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				layer2Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/new-launch-layer"))
				h.AssertNil(t, err)

				assertTarFileContents(t,
					layer2Path,
					filepath.Join(layersDir, "buildpack.id/new-launch-layer/file-from-new-launch-layer"),
					"echo text from layer 2\n")
				assertTarFileOwner(t, layer2Path, filepath.Join(layersDir, "buildpack.id/new-launch-layer"), uid, gid)
				assertAddLayerLog(t, logHandler, "buildpack.id:new-launch-layer", layer2Path)
			})

			it("adds new launch layers from a second buildpack", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				layer3Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"))
				h.AssertNil(t, err)

				assertTarFileContents(t,
					layer3Path,
					filepath.Join(layersDir, "other.buildpack.id/new-launch-layer/new-launch-layer-file"),
					"echo text from new layer\n")
				assertTarFileOwner(t, layer3Path, filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"), uid, gid)
				assertAddLayerLog(t, logHandler, "other.buildpack.id:new-launch-layer", layer3Path)
			})

			it("only creates expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				var applayer, configLayer, layer2, layer3 = 1, 1, 1, 1
				h.AssertEq(t, fakeAppImage.NumberOfAddedLayers(), applayer+configLayer+layer2+layer3)
			})

			it("only reuses expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				var launcherLayer, layer1, layer5 = 1, 1, 1
				h.AssertEq(t, len(fakeAppImage.ReusedLayers()), launcherLayer+layer1+layer5)
			})

			it("saves lifecycle metadata with layer info", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				appLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, ".hidden.txt"))
				h.AssertNil(t, err)
				appLayerSHA := h.ComputeSHA256ForFile(t, appLayerPath)

				configLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "config", "metadata.toml"))
				h.AssertNil(t, err)
				configLayerSHA := h.ComputeSHA256ForFile(t, configLayerPath)

				newLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/new-launch-layer"))
				h.AssertNil(t, err)
				newLayerSHA := h.ComputeSHA256ForFile(t, newLayerPath)

				secondBPLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"))
				h.AssertNil(t, err)
				secondBPLayerPathSHA := h.ComputeSHA256ForFile(t, secondBPLayerPath)

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
				h.AssertEq(t, meta.App[0].SHA, "sha256:"+appLayerSHA)
				h.AssertEq(t, meta.Config.SHA, "sha256:"+configLayerSHA)
				h.AssertEq(t, meta.Buildpacks[0].ID, "buildpack.id")
				h.AssertEq(t, meta.Buildpacks[0].Version, "1.2.3")
				h.AssertEq(t, meta.Buildpacks[0].Layers["launch-layer-no-local-dir"].SHA, "sha256:orig-launch-layer-no-local-dir-sha")
				h.AssertEq(t, meta.Buildpacks[0].Layers["new-launch-layer"].SHA, "sha256:"+newLayerSHA)
				h.AssertEq(t, meta.Buildpacks[1].ID, "other.buildpack.id")
				h.AssertEq(t, meta.Buildpacks[1].Version, "4.5.6")
				h.AssertEq(t, meta.Buildpacks[1].Layers["new-launch-layer"].SHA, "sha256:"+secondBPLayerPathSHA)

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
				stack = lifecycle.StackMetadata{
					RunImage: lifecycle.StackRunImageMetadata{
						Image:   "some/run",
						Mirrors: []string{"registry.example.com/some/run", "other.example.com/some/run"},
					},
				}
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

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

			when("metadata.toml is missing", func() {
				it("errors", func() {

				})
			})

			when("metadata.toml is missing bom and has empty process list", func() {
				it.Before(func() {
					err := ioutil.WriteFile(filepath.Join(layersDir, "config", "metadata.toml"), []byte(`
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
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

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

			when("there is a complete metadata.toml", func() {
				it.Before(func() {
					err := ioutil.WriteFile(filepath.Join(layersDir, "config", "metadata.toml"), []byte(`
[[processes]]
type = "web"
direct = true
command = "/web/command"
args = ["web", "command", "args"]

[[processes]]
type = "worker"
direct = false
command = "/worker/command"
args = ["worker", "command", "args"]

[[buildpacks]]
id = "buildpack.id"
version = "1.2.3"

[[buildpacks]]
id = "other.buildpack.id"
version = "4.5.6"

[[bom]]
name = "Spring Auto-reconfiguration"
version = "2.7.0"
[bom.metadata]
sha256 = "0d524877db7344ec34620f7e46254053568292f5ce514f74e3a0e9b2dbfc338b"
stacks = ["io.buildpacks.stacks.bionic", "org.cloudfoundry.stacks.cflinuxfs3"]
uri = "https://example.com"
[bom.buildpack]
id = "buildpack.id"
version = "1.2.3"

[[bom.metadata.licenses]]
type = "Apache-2.0"
`),
						os.ModePerm,
					)
					h.AssertNil(t, err)
				})

				it("combines metadata.toml with launcher config to create build label", func() {
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

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
  "processes": [
    {
      "type": "web",
      "direct": true,
      "command": "/web/command",
      "args": ["web", "command", "args"]
    },
    {
      "type": "worker",
      "direct": false,
      "command": "/worker/command",
      "args": ["worker", "command", "args"]
    }
  ]
}
`
					h.AssertJSONEq(t, expectedJSON, metadataJSON)
				})
			})

			when("there is project metadata", func() {
				it("saves metadata with project info", func() {
					fakeProjectMetadata := lifecycle.ProjectMetadata{
						Source: lifecycle.ProjectSource{
							Type: "git",
							Version: map[string]interface{}{
								"commit": "abcd1234",
							},
							Metadata: map[string]interface{}{
								"repository": "github.com/buildpack/lifecycle",
								"branch":     "master",
							},
						}}
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, fakeProjectMetadata))

					projectJSON, err := fakeAppImage.Label("io.buildpacks.project")
					h.AssertNil(t, err)

					var projectMD lifecycle.ProjectMetadata
					if err := json.Unmarshal([]byte(projectJSON), &projectMD); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}

					t.Log("adds project metadata to label")
					h.AssertEq(t, projectMD, fakeProjectMetadata)
				})
			})

			it("sets CNB_LAYERS_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				val, err := fakeAppImage.Env("CNB_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, layersDir)
			})

			it("sets CNB_APP_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				val, err := fakeAppImage.Env("CNB_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, appDir)
			})

			it("sets ENTRYPOINT to launcher", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				val, err := fakeAppImage.Entrypoint()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string{launcherConfig.Path})
			})

			it("sets empty CMD", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				val, err := fakeAppImage.Cmd()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string(nil))
			})

			it("saves run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

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
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

					assertLogEntry(t, logHandler, `*** Digest: `+fakeRemoteDigest)
				})
			})

			when("image has an ID identifier", func() {
				it("outputs the image ID", func() {
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

					assertLogEntry(t, logHandler, `*** Image ID: some-image-id`)
				})
			})

			it("outputs image names", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				assertLogEntry(t, logHandler, `*** Images (some-image-i):`)
				assertLogEntry(t, logHandler, fakeAppImage.Name())
				assertLogEntry(t, logHandler, additionalNames[0])
				assertLogEntry(t, logHandler, additionalNames[1])
			})

			when("one of the additional names fails", func() {
				it("outputs identifier and image name with error", func() {
					failingName := "not.a.tag@reference"

					err := exporter.Export(
						layersDir,
						appDir,
						fakeAppImage,
						runImageRef,
						fakeImageMetadata,
						append(additionalNames, failingName),
						launcherConfig,
						stack,
						project,
					)

					h.AssertError(t, err, fmt.Sprintf("failed to write image to the following tags: [%s:", failingName))

					assertLogEntry(t, logHandler, `*** Images (some-image-i):`)
					assertLogEntry(t, logHandler, fakeAppImage.Name())
					assertLogEntry(t, logHandler, additionalNames[0])
					assertLogEntry(t, logHandler, additionalNames[1])
					assertLogEntry(t, logHandler, fmt.Sprintf("%s - could not parse reference", failingName))
				})
			})

			when("previous image metadata is missing buildpack for reused layer", func() {
				var incompleteMetadata lifecycle.LayersMetadata

				it.Before(func() {
					h.AssertNil(t, fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata", `{"buildpacks":[{}]}`))
					h.AssertNil(t, lifecycle.DecodeLabel(fakeOriginalImage, lifecycle.LayerMetadataLabel, &incompleteMetadata))
				})

				it("returns an error", func() {
					h.AssertError(
						t,
						exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, incompleteMetadata, additionalNames, launcherConfig, stack, project),
						"cannot reuse 'buildpack.id:launch-layer-no-local-dir', previous image has no metadata for layer 'buildpack.id:launch-layer-no-local-dir'",
					)
				})
			})

			when("previous image metadata is missing reused layer", func() {
				var incompleteMetadata lifecycle.LayersMetadata

				it.Before(func() {
					_ = fakeOriginalImage.SetLabel(
						"io.buildpacks.lifecycle.metadata",
						`{"buildpacks":[{"key": "buildpack.id", "layers": {}}]}`)

					h.AssertNil(t, lifecycle.DecodeLabel(fakeOriginalImage, lifecycle.LayerMetadataLabel, &incompleteMetadata))
				})

				it("returns an error", func() {
					h.AssertError(
						t,
						exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, incompleteMetadata, additionalNames, launcherConfig, stack, project),
						"cannot reuse 'buildpack.id:launch-layer-no-local-dir', previous image has no metadata for layer 'buildpack.id:launch-layer-no-local-dir'",
					)
				})
			})

			it("saves the image for all provided additionalNames", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))
				h.AssertContains(t, fakeAppImage.SavedNames(), append(additionalNames, fakeAppImage.Name())...)
			})
		})

		when("previous slice image doesn't exist", func() {
			var (
				nonExistingOriginalImage *fakes.Image
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "app-slices", "layers"), layersDir)
				var err error
				appDir, err = filepath.Abs(filepath.Join(layersDir, "app"))
				h.AssertNil(t, err)

				// TODO : this is an hacky way to create a non-existing image and should be improved in imgutil
				nonExistingOriginalImage = fakes.NewImage("app/original-image", "", nil)
				nonExistingOriginalImage.Delete()
			})

			it.After(func() {
				nonExistingOriginalImage.Cleanup()
			})

			it("create a slice layer on the Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				sliceLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, "static", "assets", "config.txt"))
				h.AssertNil(t, err)

				assertTarFileExists(t, sliceLayerPath, filepath.Join(appDir, "static", "assets", "config.txt"), true)
				assertTarFileExists(t, sliceLayerPath, filepath.Join(appDir, "static", "assets", "logo.svg"), true)

				appLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, ".hidden.txt"))
				h.AssertNil(t, err)

				assertTarFileExists(t, appLayerPath, filepath.Join(appDir, "static", "assets", "config.txt"), false)
				assertTarFileExists(t, appLayerPath, filepath.Join(appDir, "static", "assets", "logo.svg"), false)
				assertTarFileExists(t, appLayerPath, filepath.Join(appDir, "static", "misc", "resources"), false)
				assertTarFileExists(t, appLayerPath, filepath.Join(appDir, "test_app.sh"), true)
				assertTarFileExists(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), true)

				assertLogEntry(t, logHandler, "Adding 4/4 app layer(s)")
			})
		})

		when("previous image doesn't exist", func() {
			var (
				nonExistingOriginalImage *fakes.Image
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "previous-image-not-exist", "layers"), layersDir)
				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "previous-image-not-exist", "layers", "app"))
				h.AssertNil(t, err)

				// TODO : this is an hacky way to create a non-existing image and should be improved in imgutil
				nonExistingOriginalImage = fakes.NewImage("app/original-image", "", nil)
				nonExistingOriginalImage.Delete()
			})

			it.After(func() {
				nonExistingOriginalImage.Cleanup()
			})

			it("creates app layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				appLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, ".hidden.txt"))
				h.AssertNil(t, err)

				assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
				assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
				assertLogEntry(t, logHandler, "Adding 1/1 app layer(s)")
			})

			when("app dir is relative", func() {
				it("creates app layer on Run image", func() {
					cwd, err := os.Getwd()
					h.AssertNil(t, err)

					relAppDir, err := filepath.Rel(cwd, appDir)
					h.AssertNil(t, err)

					h.AssertNil(t, exporter.Export(layersDir, relAppDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

					appLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, ".hidden.txt"))
					h.AssertNil(t, err)

					assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
					assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
					assertLogEntry(t, logHandler, "Adding 1/1 app layer(s)")
				})
			})

			it("creates config layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				configLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "config", "metadata.toml"))
				h.AssertNil(t, err)

				assertTarFileContents(t,
					configLayerPath,
					filepath.Join(layersDir, "config/metadata.toml"),
					"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
				)
				assertTarFileOwner(t, configLayerPath, filepath.Join(layersDir, "config"), uid, gid)
				assertAddLayerLog(t, logHandler, "config", configLayerPath)
			})

			it("creates a launcher layer", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				launcherLayerPath, err := fakeAppImage.FindLayerWithPath(launcherConfig.Path)
				h.AssertNil(t, err)
				assertTarFileContents(t,
					launcherLayerPath,
					launcherConfig.Path,
					"some-launcher")
				assertTarFileOwner(t, launcherLayerPath, launcherConfig.Path, uid, gid)
				assertAddLayerLog(t, logHandler, "launcher", launcherLayerPath)
			})

			it("adds launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				layer1Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer1"))
				h.AssertNil(t, err)
				assertTarFileContents(t,
					layer1Path,
					filepath.Join(layersDir, "buildpack.id/layer1/file-from-layer-1"),
					"echo text from layer 1\n")
				assertTarFileOwner(t, layer1Path, filepath.Join(layersDir, "buildpack.id/layer1"), uid, gid)
				assertAddLayerLog(t, logHandler, "buildpack.id:layer1", layer1Path)

				layer2Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				h.AssertNil(t, err)
				assertTarFileContents(t,
					layer2Path,
					filepath.Join(layersDir, "buildpack.id/layer2/file-from-layer-2"),
					"echo text from layer 2\n")
				assertTarFileOwner(t, layer2Path, filepath.Join(layersDir, "buildpack.id/layer2"), uid, gid)
				assertAddLayerLog(t, logHandler, "buildpack.id:layer2", layer2Path)
			})

			it("only creates expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				var applayer, configLayer, launcherLayer, layer1, layer2 = 1, 1, 1, 1, 1
				h.AssertEq(t, fakeAppImage.NumberOfAddedLayers(), applayer+configLayer+launcherLayer+layer1+layer2)
			})

			it("saves metadata with layer info", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				appLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(appDir, ".hidden.txt"))
				h.AssertNil(t, err)
				appLayerSHA := h.ComputeSHA256ForFile(t, appLayerPath)

				configLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "config", "metadata.toml"))
				h.AssertNil(t, err)
				configLayerSHA := h.ComputeSHA256ForFile(t, configLayerPath)

				launcherLayerPath, err := fakeAppImage.FindLayerWithPath(launcherConfig.Path)
				h.AssertNil(t, err)
				launcherLayerSHA := h.ComputeSHA256ForFile(t, launcherLayerPath)

				layer1Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer1"))
				h.AssertNil(t, err)
				buildpackLayer1SHA := h.ComputeSHA256ForFile(t, layer1Path)

				layer2Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				h.AssertNil(t, err)
				buildpackLayer2SHA := h.ComputeSHA256ForFile(t, layer2Path)

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
				h.AssertEq(t, meta.App[0].SHA, "sha256:"+appLayerSHA)
				h.AssertEq(t, meta.Config.SHA, "sha256:"+configLayerSHA)
				h.AssertEq(t, meta.Launcher.SHA, "sha256:"+launcherLayerSHA)
				h.AssertEq(t, meta.Buildpacks[0].ID, "buildpack.id")
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer1"].SHA, "sha256:"+buildpackLayer1SHA)
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer2"].SHA, "sha256:"+buildpackLayer2SHA)

				t.Log("adds buildpack layer metadata to label")
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer1"].Data, map[string]interface{}{
					"mykey": "new val",
				})

				t.Log("defaults to nil store")
				h.AssertNil(t, meta.Buildpacks[0].Store)
			})

			when("there are store.toml files", func() {
				it.Before(func() {
					path := filepath.Join(layersDir, "buildpack.id", "store.toml")
					h.AssertNil(t, ioutil.WriteFile(path, []byte("[metadata]\n  key = \"val\""), 0777))
				})

				it("saves store metadata", func() {
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

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
					fakeProjectMetadata := lifecycle.ProjectMetadata{
						Source: lifecycle.ProjectSource{
							Type: "git",
							Version: map[string]interface{}{
								"commit": "abcd1234",
							},
							Metadata: map[string]interface{}{
								"repository": "github.com/buildpack/lifecycle",
								"branch":     "master",
							},
						}}
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, fakeProjectMetadata))

					projectJSON, err := fakeAppImage.Label("io.buildpacks.project")
					h.AssertNil(t, err)

					var projectMD lifecycle.ProjectMetadata
					if err := json.Unmarshal([]byte(projectJSON), &projectMD); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}

					t.Log("adds project metadata to label")
					h.AssertEq(t, projectMD, fakeProjectMetadata)
				})
			})

			it("sets CNB_LAYERS_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				val, err := fakeAppImage.Env("CNB_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, layersDir)
			})

			it("sets CNB_APP_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				val, err := fakeAppImage.Env("CNB_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, appDir)
			})

			it("sets ENTRYPOINT to launcher", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				val, err := fakeAppImage.Entrypoint()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string{launcherConfig.Path})
			})

			it("sets empty CMD", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))

				val, err := fakeAppImage.Cmd()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string(nil))
			})

			it("saves the image for all provided additionalNames", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project))
				h.AssertContains(t, fakeAppImage.SavedNames(), append(additionalNames, fakeAppImage.Name())...)
			})
		})

		when("buildpack requires an escaped id", func() {
			var (
				fakeOriginalImage *fakes.Image
				fakeImageMetadata lifecycle.LayersMetadata
			)

			it.Before(func() {
				exporter.Buildpacks = []lifecycle.Buildpack{{ID: "some/escaped/bp/id"}}

				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "escaped-bpid", "layers"), layersDir)

				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "escaped-bpid", "layers", "app"))
				h.AssertNil(t, err)

				fakeOriginalImage = fakes.NewImage("app/original", "original-top-sha", local.IDIdentifier{ImageID: "run-digest"})
				h.AssertNil(t, fakeOriginalImage.SetLabel(
					"io.buildpacks.lifecycle.metadata",
					`{"buildpacks":[{"key": "some/escaped/bp/id", "layers": {"layer": {"sha": "original-layer-sha"}}}]}`,
				))

				h.AssertNil(t, lifecycle.DecodeLabel(fakeOriginalImage, lifecycle.LayerMetadataLabel, &fakeImageMetadata))
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("exports layers from the escaped id path", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

				layerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "some_escaped_bp_id/some-layer"))
				h.AssertNil(t, err)

				assertTarFileContents(t,
					layerPath,
					filepath.Join(layersDir, "some_escaped_bp_id/some-layer/some-file"),
					"some-file-contents\n")
				assertTarFileOwner(t, layerPath, filepath.Join(layersDir, "some_escaped_bp_id/some-layer/some-file"), uid, gid)
				assertAddLayerLog(t, logHandler, "some/escaped/bp/id:some-layer", layerPath)
			})

			it("exports buildpack metadata with unescaped id", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project))

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
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "bad-layer", "layers"), layersDir)

				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "bad-layer", "layers", "app"))
				h.AssertNil(t, err)

				// TODO : this is an hacky way to create a non-existing image and should be improved in imgutil
				nonExistingOriginalImage = fakes.NewImage("app/original-image", "", nil)
				nonExistingOriginalImage.Delete()
			})

			it.After(func() {
				nonExistingOriginalImage.Cleanup()
			})

			it("returns an error", func() {
				h.AssertError(
					t,
					exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, lifecycle.LayersMetadata{}, additionalNames, launcherConfig, stack, project),
					"failed to parse metadata for layers '[buildpack.id:bad-layer]'",
				)
			})
		})

		when("there is a launch=true cache=true layer without contents", func() {
			var (
				fakeOriginalImage *fakes.Image
				fakeImageMetadata lifecycle.LayersMetadata
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "cache-layer-no-contents", "layers"), layersDir)
				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "cache-layer-no-contents", "layers", "app"))
				h.AssertNil(t, err)

				fakeOriginalImage = fakes.NewImage(
					"app/original-image",
					"original-top-layer-sha",
					local.IDIdentifier{ImageID: "some-original-image-id"},
				)
				_ = fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata", `{
  "buildpacks": [
    {
      "key": "buildpack.id",
      "layers": {
        "cache-layer-no-contents": {
          "sha": "some-sha",
          "cache": true
        }
      }
    }
  ]
}`)

				h.AssertNil(t, lifecycle.DecodeLabel(fakeOriginalImage, lifecycle.LayerMetadataLabel, &fakeImageMetadata))
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("returns an error", func() {
				h.AssertError(
					t,
					exporter.Export(layersDir, appDir, fakeAppImage, runImageRef, fakeImageMetadata, additionalNames, launcherConfig, stack, project),
					"layer 'buildpack.id:cache-layer-no-contents' is cache=true but has no contents",
				)
			})
		})
	})

	when("#Cache", func() {
		var (
			tmpDir                 string
			cacheDir               string
			testCache              lifecycle.Cache
			layersDir              string
			cacheTrueLayerSHA      string
			otherBuildpackLayerSHA string
			exporter               *lifecycle.Exporter
		)

		it.Before(func() {
			var err error

			tmpDir, err = ioutil.TempDir("", "lifecycle.cacher.layer")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)

			testCache, err = cache.NewVolumeCache(cacheDir)
			h.AssertNil(t, err)

			exporter = &lifecycle.Exporter{
				ArtifactsDir: tmpDir,
				Buildpacks: []lifecycle.Buildpack{
					{ID: "buildpack.id"},
					{ID: "other.buildpack.id"},
				},
				Logger: &log.Logger{Handler: &discard.Handler{}},
				UID:    1234,
				GID:    4321,
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(cacheDir))
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		when("the layers are valid", func() {
			it.Before(func() {
				layersDir = filepath.Join("testdata", "cacher", "layers")
				cacheTrueLayerSHA = "sha256:" + h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "buildpack.id/cache-true-layer"), 1234, 4321)
				otherBuildpackLayerSHA = "sha256:" + h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/other-buildpack-layer"), 1234, 4321)
			})

			when("there is no previous cache", func() {
				it("adds layers with 'cache=true' to the cache", func() {
					err := exporter.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					assertTarFileContents(
						t,
						filepath.Join(cacheDir, "committed", cacheTrueLayerSHA+".tar"),
						filepath.Join(layersDir, "buildpack.id/cache-true-layer/file-from-cache-true-layer"),
						"file-from-cache-true-contents",
					)

					assertTarFileContents(
						t,
						filepath.Join(cacheDir, "committed", otherBuildpackLayerSHA+".tar"),
						filepath.Join(layersDir, "other.buildpack.id/other-buildpack-layer/other-buildpack-layer-file"),
						"other-buildpack-layer-contents",
					)
				})

				it("sets the uid and gid of the layer contents", func() {
					err := exporter.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					assertTarFileOwner(
						t,
						filepath.Join(cacheDir, "committed", cacheTrueLayerSHA+".tar"),
						filepath.Join(layersDir, "buildpack.id/cache-true-layer/file-from-cache-true-layer"),
						1234,
						4321,
					)

					assertTarFileOwner(
						t,
						filepath.Join(cacheDir, "committed", otherBuildpackLayerSHA+".tar"),
						filepath.Join(layersDir, "other.buildpack.id/other-buildpack-layer/other-buildpack-layer-file"),
						1234,
						4321,
					)
				})

				it("sets cache metadata", func() {
					err := exporter.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					metadata, err := testCache.RetrieveMetadata()
					h.AssertNil(t, err)

					t.Log("adds layer shas to metadata")
					h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].SHA, cacheTrueLayerSHA)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Launch, true)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Build, false)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Cache, true)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Data, map[string]interface{}{
						"cache-true-key": "cache-true-val",
					})
				})

				it("doesn't export uncached layers", func() {
					err := exporter.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					matches, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
					h.AssertNil(t, err)
					h.AssertEq(t, len(matches), 3)
				})
			})

			when("there are previously cached layers", func() {
				var (
					computedReusableLayerSHA string
					metadataTemplate         string
				)

				it.Before(func() {
					computedReusableLayerSHA = "sha256:" + h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "buildpack.id/cache-true-no-sha-layer"), 1234, 4321)
					metadataTemplate = `{
					"buildpacks": [
					 {
					   "key": "buildpack.id",
					   "layers": {
					     "cache-true-layer": {
					       "cache": true,
					       "sha": "%s",
					       "data": {"old":"data"}
					     },
					     "cache-true-no-sha-layer": {
					       "cache": true,
					       "sha": "%s"
					     }
					   }
					 }
					]
					}`
				})

				when("the SHAs match", func() {
					it.Before(func() {
						previousCache, err := cache.NewVolumeCache(cacheDir)
						h.AssertNil(t, err)

						err = exporter.Cache(layersDir, previousCache)
						h.AssertNil(t, err)

						testCache, err = cache.NewVolumeCache(cacheDir)
						h.AssertNil(t, err)
					})

					it("reuses layers when the calculated sha matches previous metadata", func() {
						previousLayers, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)

						err = exporter.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						reusedLayers, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)

						h.AssertEq(t, previousLayers, reusedLayers)
					})

					it("sets cache metadata", func() {
						err := exporter.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						metadata, err := testCache.RetrieveMetadata()
						h.AssertNil(t, err)

						t.Log("adds layer shas to metadata")
						h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].SHA, cacheTrueLayerSHA)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Launch, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Data, map[string]interface{}{
							"cache-true-key": "cache-true-val",
						})

						h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].SHA, computedReusableLayerSHA)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Launch, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Data, map[string]interface{}{
							"cache-true-no-sha-key": "cache-true-no-sha-val",
						})

						h.AssertEq(t, metadata.Buildpacks[1].ID, "other.buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].SHA, otherBuildpackLayerSHA)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Launch, true)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Data, map[string]interface{}{
							"other-buildpack-key": "other-buildpack-val",
						})
					})
				})

				when("the shas don't match", func() {
					it.Before(func() {
						err := ioutil.WriteFile(
							filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
							[]byte(fmt.Sprintf(metadataTemplate, "different-sha", "not-the-sha-you-want")),
							0666,
						)
						h.AssertNil(t, err)

						err = ioutil.WriteFile(
							filepath.Join(cacheDir, "committed", "some-layer.tar"),
							[]byte("some data"),
							0666,
						)
						h.AssertNil(t, err)
					})

					it("doesn't reuse layers", func() {
						err := exporter.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						matches, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)
						h.AssertEq(t, len(matches), 3)

						for _, m := range matches {
							if strings.Contains(m, "some-layer.tar") {
								t.Fatal("expected layer 'some-layer.tar' not to exist")
							}
						}
					})
				})
			})
		})

		when("there is a cache=true layer without contents", func() {
			it.Before(func() {
				layersDir = filepath.Join("testdata", "cacher", "invalid-layers")

				err := ioutil.WriteFile(
					filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
					[]byte("{}"),
					0666,
				)
				h.AssertNil(t, err)
			})

			it("fails", func() {
				err := exporter.Cache(layersDir, testCache)
				h.AssertError(t, err, "failed to cache layer 'buildpack.id:cache-true-no-contents' because it has no contents")
			})
		})
	})
}

func assertAddLayerLog(t *testing.T, logHandler *memory.Handler, name, layerPath string) {
	t.Helper()
	layerSHA := h.ComputeSHA256ForFile(t, layerPath)
	assertLogEntry(t, logHandler, fmt.Sprintf("Adding layer '%s'", name))
	assertLogEntry(t, logHandler, fmt.Sprintf("Layer '%s' SHA: sha256:%s", name, layerSHA))
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

func assertReuseLayerLog(t *testing.T, logHandler *memory.Handler, name, sha string) {
	t.Helper()
	assertLogEntry(t, logHandler, fmt.Sprintf("Reusing layer '%s'", name))
	assertLogEntry(t, logHandler, fmt.Sprintf("Layer '%s' SHA: sha256:%s", name, sha))
}

func assertTarFileContents(t *testing.T, tarfile, path, expected string) {
	t.Helper()
	exist, contents := tarFileContext(t, tarfile, path)
	if !exist {
		t.Fatalf("%s does not exist in %s", path, tarfile)
	}
	h.AssertEq(t, contents, expected)
}

func assertTarFileExists(t *testing.T, tarfile, path string, expected bool) {
	t.Helper()
	exist, _ := tarFileContext(t, tarfile, path)
	if !exist {
		h.AssertEq(t, false, expected)
	}
	h.AssertEq(t, exist, expected)
}

func tarFileContext(t *testing.T, tarfile, path string) (exist bool, contents string) {
	t.Helper()
	r, err := os.Open(tarfile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name == path {
			buf, err := ioutil.ReadAll(tr)
			h.AssertNil(t, err)
			return true, string(buf)
		}
	}
	return false, ""
}

func assertTarFileOwner(t *testing.T, tarfile, path string, expectedUID, expectedGID int) {
	t.Helper()
	var foundPath bool
	r, err := os.Open(tarfile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name == path {
			foundPath = true
			if header.Uid != expectedUID {
				t.Fatalf("expected all entries in `%s` to have uid '%d', but '%s' has '%d'", tarfile, expectedUID, header.Name, header.Uid)
			}
			if header.Gid != expectedGID {
				t.Fatalf("expected all entries in `%s` to have gid '%d', got '%d'", tarfile, expectedGID, header.Gid)
			}
		}
	}
	if !foundPath {
		t.Fatalf("%s does not exist in %s", path, tarfile)
	}
}
