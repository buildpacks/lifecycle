package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpack/imgutil/fakes"
	"github.com/buildpack/imgutil/local"
	"github.com/buildpack/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/internal/mocks"
	"github.com/buildpack/lifecycle/metadata"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestExporter(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Exporter", testExporter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	var (
		exporter        *lifecycle.Exporter
		fakeAppImage    *fakes.Image
		outLog          *bytes.Buffer
		layersDir       string
		tmpDir          string
		appDir          string
		launcherConfig  lifecycle.LauncherConfig
		uid             = 1234
		gid             = 4321
		stack           = metadata.StackMetadata{}
		additionalNames []string
	)

	it.Before(func() {
		outLog = &bytes.Buffer{}

		tmpDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
		h.AssertNil(t, err)

		launcherPath, err := filepath.Abs(filepath.Join("testdata", "exporter", "launcher"))
		h.AssertNil(t, err)

		launcherConfig = lifecycle.LauncherConfig{
			Path: launcherPath,
			Metadata: metadata.LauncherMetadata{
				Version: "1.2.3",
				Source: metadata.SourceMetadata{
					Git: metadata.GitMetadata{
						Repository: "github.com/buildpack/lifecycle",
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
			Logger: mocks.NewMockLogger(io.MultiWriter(outLog, it.Out())),
			UID: uid,
			GID: gid,
		}
	})

	it.After(func() {
		fakeAppImage.Cleanup()

		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatal(err)
		}
	})

	when("#Export", func() {
		when("previous image exists", func() {
			var (
				fakeOriginalImage *fakes.Image
				fakeImageMetadata metadata.LayersMetadata
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
					fmt.Sprintf(`{
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

				fakeImageMetadata, err = metadata.GetLayersMetdata(fakeOriginalImage)
				h.AssertNil(t, err)
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("creates app layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				appLayerPath := fakeAppImage.AppLayerPath()

				assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
				assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
				assertAddLayerLog(t, *outLog, "app", appLayerPath)
			})

			it("creates config layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				configLayerPath := fakeAppImage.ConfigLayerPath()

				assertTarFileContents(t,
					configLayerPath,
					filepath.Join(layersDir, "config", "metadata.toml"),
					"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
				)
				assertTarFileOwner(t, configLayerPath, filepath.Join(layersDir, "config"), uid, gid)
				assertAddLayerLog(t, *outLog, "config", configLayerPath)
			})

			it("reuses launcher layer if the sha matches the sha in the metadata", func() {
				launcherLayerSHA := h.ComputeSHA256ForPath(t, launcherConfig.Path, uid, gid)
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))
				h.AssertContains(t, fakeAppImage.ReusedLayers(), "sha256:"+launcherLayerSHA)
				assertReuseLayerLog(t, *outLog, "launcher", launcherLayerSHA)
			})

			it("reuses launch layers when only layer.toml is present", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				h.AssertContains(t, fakeAppImage.ReusedLayers(), "sha256:orig-launch-layer-no-local-dir-sha")
				assertReuseLayerLog(t, *outLog, "buildpack.id:launch-layer-no-local-dir", "orig-launch-layer-no-local-dir-sha")
			})

			it("reuses cached launch layers if the local sha matches the sha in the metadata", func() {
				layer5sha := h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/local-reusable-layer"), uid, gid)

				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				h.AssertContains(t, fakeAppImage.ReusedLayers(), "sha256:"+layer5sha)
				assertReuseLayerLog(t, *outLog, "other.buildpack.id:local-reusable-layer", layer5sha)
			})

			it("adds new launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				layer2Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/new-launch-layer"))
				h.AssertNil(t, err)

				assertTarFileContents(t,
					layer2Path,
					filepath.Join(layersDir, "buildpack.id/new-launch-layer/file-from-new-launch-layer"),
					"echo text from layer 2\n")
				assertTarFileOwner(t, layer2Path, filepath.Join(layersDir, "buildpack.id/new-launch-layer"), uid, gid)
				assertAddLayerLog(t, *outLog, "buildpack.id:new-launch-layer", layer2Path)
			})

			it("adds new launch layers from a second buildpack", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				layer3Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"))
				h.AssertNil(t, err)

				assertTarFileContents(t,
					layer3Path,
					filepath.Join(layersDir, "other.buildpack.id/new-launch-layer/new-launch-layer-file"),
					"echo text from new layer\n")
				assertTarFileOwner(t, layer3Path, filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"), uid, gid)
				assertAddLayerLog(t, *outLog, "other.buildpack.id:new-launch-layer", layer3Path)
			})

			it("only creates expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				var applayer, configLayer, layer2, layer3 = 1, 1, 1, 1
				h.AssertEq(t, fakeAppImage.NumberOfAddedLayers(), applayer+configLayer+layer2+layer3)
			})

			it("only reuses expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				var launcherLayer, layer1, layer5 = 1, 1, 1
				h.AssertEq(t, len(fakeAppImage.ReusedLayers()), launcherLayer+layer1+layer5)
			})

			it("saves lifecycle metadata with layer info", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				appLayerPath := fakeAppImage.AppLayerPath()
				appLayerSHA := h.ComputeSHA256ForFile(t, appLayerPath)

				configLayerPath := fakeAppImage.ConfigLayerPath()
				configLayerSHA := h.ComputeSHA256ForFile(t, configLayerPath)

				newLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/new-launch-layer"))
				h.AssertNil(t, err)
				newLayerSHA := h.ComputeSHA256ForFile(t, newLayerPath)

				secondBPLayerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"))
				h.AssertNil(t, err)
				secondBPLayerPathSHA := h.ComputeSHA256ForFile(t, secondBPLayerPath)

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta metadata.LayersMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				t.Log("adds run image metadata to label")
				h.AssertEq(t, meta.RunImage.TopLayer, "some-top-layer-sha")
				h.AssertEq(t, meta.RunImage.Reference, "some-image-id")

				t.Log("adds layer shas to metadata label")
				h.AssertEq(t, meta.App.SHA, "sha256:"+appLayerSHA)
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
				stack = metadata.StackMetadata{
					RunImage: metadata.StackRunImageMetadata{
						Image:   "some/run",
						Mirrors: []string{"registry.example.com/some/run", "other.example.com/some/run"},
					},
				}
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta metadata.LayersMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}
				h.AssertNil(t, err)
				h.AssertEq(t, meta.Stack.RunImage.Image, "some/run")
				h.AssertEq(t, meta.Stack.RunImage.Mirrors, []string{"registry.example.com/some/run", "other.example.com/some/run"})
			})

			when("metadata.toml does not include BOM", func() {
				it.Before(func() {
					err := ioutil.WriteFile(filepath.Join(layersDir, "config", "metadata.toml"), []byte(`
[[processes]]
  type = "web"
  command = "npm start"
`),
						os.ModePerm,
					)
					h.AssertNil(t, err)
				})

				it("BOM is not present in the label", func() {
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

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
        "repository": "github.com/buildpack/lifecycle",
        "commit": "asdf1234"
      }
    }
  }
}
`
					h.AssertJSONEq(t, expectedJSON, metadataJSON)
				})
			})

			when("metadata.toml includes BOM", func() {
				it.Before(func() {
					err := ioutil.WriteFile(filepath.Join(layersDir, "config", "metadata.toml"), []byte(`
[[processes]]
type = "web"
command = "npm start"

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

				it("saves BOM metadata to the resulting image", func() {
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

					metadataJSON, err := fakeAppImage.Label("io.buildpacks.build.metadata")
					h.AssertNil(t, err)

					expectedJSON := `
{
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
        "repository": "github.com/buildpack/lifecycle",
        "commit": "asdf1234"
      }
    }
  }
}
`

					h.AssertJSONEq(t, expectedJSON, metadataJSON)
				})
			})

			it("saves buildpacks to build metadata label", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

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
        "repository": "github.com/buildpack/lifecycle",
        "commit": "asdf1234"
      }
    }
  }
}
`
				h.AssertJSONEq(t, expectedJSON, metadataJSON)
			})

			it("sets CNB_LAYERS_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				val, err := fakeAppImage.Env("CNB_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, layersDir)
			})

			it("sets CNB_APP_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				val, err := fakeAppImage.Env("CNB_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, appDir)
			})

			it("sets ENTRYPOINT to launcher", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				val, err := fakeAppImage.Entrypoint()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string{launcherConfig.Path})
			})

			it("sets empty CMD", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				val, err := fakeAppImage.Cmd()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string(nil))
			})

			it("saves run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

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
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

					h.AssertStringContains(t,
						outLog.String(),
						`*** Digest: `+fakeRemoteDigest,
					)
				})
			})

			when("image has an ID identifier", func() {
				it("outputs the image ID", func() {
					h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

					h.AssertStringContains(t,
						outLog.String(),
						`*** Image ID: some-image-id`,
					)
				})
			})

			it("outputs image names", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				h.AssertStringContains(t,
					outLog.String(),
					fmt.Sprintf(`*** Images:
      %s - succeeded
      %s - succeeded
      %s - succeeded
`,
						fakeAppImage.Name(),
						additionalNames[0],
						additionalNames[1],
					),
				)
			})

			when("one of the additional names fails", func() {
				it("outputs identifier and image name with error", func() {
					failingName := "not.a.tag@reference"

					err := exporter.Export(
						layersDir,
						appDir,
						fakeAppImage,
						fakeImageMetadata,
						append(additionalNames, failingName),
						launcherConfig,
						stack,
					)

					h.AssertError(t, err, fmt.Sprintf("failed to write image to the following tags: [%s]", failingName))

					h.AssertStringContains(t,
						outLog.String(),
						fmt.Sprintf(
							`*** Images:
      %s - succeeded
      %s - succeeded
      %s - succeeded
      %s - could not parse reference

*** Image ID: some-image-id`,
							fakeAppImage.Name(),
							additionalNames[0],
							additionalNames[1],
							failingName,
						),
					)
				})
			})

			when("previous image metadata is missing buildpack for reused layer", func() {
				it.Before(func() {
					_ = fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata", `{"buildpacks":[{}]}`)

					var err error
					fakeImageMetadata, err = metadata.GetLayersMetdata(fakeOriginalImage)
					h.AssertNil(t, err)
				})

				it("returns an error", func() {
					h.AssertError(
						t,
						exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack),
						"cannot reuse 'buildpack.id:launch-layer-no-local-dir', previous image has no metadata for layer 'buildpack.id:launch-layer-no-local-dir'",
					)
				})
			})

			when("previous image metadata is missing reused layer", func() {
				it.Before(func() {
					_ = fakeOriginalImage.SetLabel(
						"io.buildpacks.lifecycle.metadata",
						`{"buildpacks":[{"key": "buildpack.id", "layers": {}}]}`)

					var err error
					fakeImageMetadata, err = metadata.GetLayersMetdata(fakeOriginalImage)
					h.AssertNil(t, err)
				})

				it("returns an error", func() {
					h.AssertError(
						t,
						exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack),
						"cannot reuse 'buildpack.id:launch-layer-no-local-dir', previous image has no metadata for layer 'buildpack.id:launch-layer-no-local-dir'",
					)
				})
			})

			it("saves the image for all provided additionalNames", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))
				h.AssertContains(t, fakeAppImage.SavedNames(), append(additionalNames, fakeAppImage.Name())...)
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
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				appLayerPath := fakeAppImage.AppLayerPath()

				assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
				assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
				assertAddLayerLog(t, *outLog, "app", appLayerPath)
			})

			it("creates config layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				configLayerPath := fakeAppImage.ConfigLayerPath()

				assertTarFileContents(t,
					configLayerPath,
					filepath.Join(layersDir, "config/metadata.toml"),
					"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
				)
				assertTarFileOwner(t, configLayerPath, filepath.Join(layersDir, "config"), uid, gid)
				assertAddLayerLog(t, *outLog, "config", configLayerPath)
			})

			it("creates a launcher layer", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				launcherLayerPath, err := fakeAppImage.FindLayerWithPath(launcherConfig.Path)
				h.AssertNil(t, err)
				assertTarFileContents(t,
					launcherLayerPath,
					launcherConfig.Path,
					"some-launcher")
				assertTarFileOwner(t, launcherLayerPath, launcherConfig.Path, uid, gid)
				assertAddLayerLog(t, *outLog, "launcher", launcherLayerPath)
			})

			it("adds launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				layer1Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer1"))
				h.AssertNil(t, err)
				assertTarFileContents(t,
					layer1Path,
					filepath.Join(layersDir, "buildpack.id/layer1/file-from-layer-1"),
					"echo text from layer 1\n")
				assertTarFileOwner(t, layer1Path, filepath.Join(layersDir, "buildpack.id/layer1"), uid, gid)
				assertAddLayerLog(t, *outLog, "buildpack.id:layer1", layer1Path)

				layer2Path, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				h.AssertNil(t, err)
				assertTarFileContents(t,
					layer2Path,
					filepath.Join(layersDir, "buildpack.id/layer2/file-from-layer-2"),
					"echo text from layer 2\n")
				assertTarFileOwner(t, layer2Path, filepath.Join(layersDir, "buildpack.id/layer2"), uid, gid)
				assertAddLayerLog(t, *outLog, "buildpack.id:layer2", layer2Path)
			})

			it("only creates expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				var applayer, configLayer, launcherLayer, layer1, layer2 = 1, 1, 1, 1, 1
				h.AssertEq(t, fakeAppImage.NumberOfAddedLayers(), applayer+configLayer+launcherLayer+layer1+layer2)
			})

			it("saves metadata with layer info", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				appLayerPath := fakeAppImage.AppLayerPath()
				appLayerSHA := h.ComputeSHA256ForFile(t, appLayerPath)

				configLayerPath := fakeAppImage.ConfigLayerPath()
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

				var meta metadata.LayersMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				t.Log("adds run image metadata to label")
				h.AssertEq(t, meta.RunImage.TopLayer, "some-top-layer-sha")
				h.AssertEq(t, meta.RunImage.Reference, "some-image-id")

				t.Log("adds layer shas to metadata label")
				h.AssertEq(t, meta.App.SHA, "sha256:"+appLayerSHA)
				h.AssertEq(t, meta.Config.SHA, "sha256:"+configLayerSHA)
				h.AssertEq(t, meta.Launcher.SHA, "sha256:"+launcherLayerSHA)
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer1"].SHA, "sha256:"+buildpackLayer1SHA)
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer2"].SHA, "sha256:"+buildpackLayer2SHA)

				t.Log("adds buildpack layer metadata to label")
				h.AssertEq(t, meta.Buildpacks[0].Layers["layer1"].Data, map[string]interface{}{
					"mykey": "new val",
				})
			})

			it("sets CNB_LAYERS_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				val, err := fakeAppImage.Env("CNB_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, layersDir)
			})

			it("sets CNB_APP_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				val, err := fakeAppImage.Env("CNB_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, appDir)
			})

			it("sets ENTRYPOINT to launcher", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				val, err := fakeAppImage.Entrypoint()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string{launcherConfig.Path})
			})

			it("sets empty CMD", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))

				val, err := fakeAppImage.Cmd()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string(nil))
			})

			it("saves the image for all provided additionalNames", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack))
				h.AssertContains(t, fakeAppImage.SavedNames(), append(additionalNames, fakeAppImage.Name())...)
			})
		})

		when("buildpack requires an escaped id", func() {
			var (
				fakeOriginalImage *fakes.Image
				fakeImageMetadata metadata.LayersMetadata
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

				fakeImageMetadata, err = metadata.GetLayersMetdata(fakeOriginalImage)
				h.AssertNil(t, err)
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("exports layers from the escaped id path", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				layerPath, err := fakeAppImage.FindLayerWithPath(filepath.Join(layersDir, "some_escaped_bp_id/some-layer"))
				h.AssertNil(t, err)

				assertTarFileContents(t,
					layerPath,
					filepath.Join(layersDir, "some_escaped_bp_id/some-layer/some-file"),
					"some-file-contents\n")
				assertTarFileOwner(t, layerPath, filepath.Join(layersDir, "some_escaped_bp_id/some-layer/some-file"), uid, gid)
				assertAddLayerLog(t, *outLog, "some/escaped/bp/id:some-layer", layerPath)
			})

			it("exports buildpack metadata with unescaped id", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack))

				metadataJSON, err := fakeAppImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta metadata.LayersMetadata
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
					exporter.Export(layersDir, appDir, fakeAppImage, metadata.LayersMetadata{}, additionalNames, launcherConfig, stack),
					"failed to parse metadata for layers '[buildpack.id:bad-layer]'",
				)
			})
		})

		when("there is a launch=true cache=true layer without contents", func() {
			var (
				fakeOriginalImage *fakes.Image
				fakeImageMetadata metadata.LayersMetadata
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

				fakeImageMetadata, err = metadata.GetLayersMetdata(fakeOriginalImage)
				h.AssertNil(t, err)
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("returns an error", func() {
				h.AssertError(
					t,
					exporter.Export(layersDir, appDir, fakeAppImage, fakeImageMetadata, additionalNames, launcherConfig, stack),
					"layer 'buildpack.id:cache-layer-no-contents' is cache=true but has no contents",
				)
			})
		})
	})
}

func assertAddLayerLog(t *testing.T, stdout bytes.Buffer, name, layerPath string) {
	t.Helper()
	layerSHA := h.ComputeSHA256ForFile(t, layerPath)

	expected := fmt.Sprintf("Exporting layer '%s' with SHA sha256:%s", name, layerSHA)
	h.AssertStringContains(t, stdout.String(), expected)
}

func assertReuseLayerLog(t *testing.T, stdout bytes.Buffer, name, sha string) {
	t.Helper()
	expected := fmt.Sprintf("Reusing layer '%s' with SHA sha256:%s", name, sha)
	h.AssertStringContains(t, stdout.String(), expected)
}

func assertTarFileContents(t *testing.T, tarfile, path, expected string) {
	t.Helper()
	exist, contents := tarFileContext(t, tarfile, path)
	if !exist {
		t.Fatalf("%s does not exist in %s", path, tarfile)
	}
	h.AssertEq(t, contents, expected)
}

func tarFileContext(t *testing.T, tarfile, path string) (exist bool, contents string) {
	t.Helper()
	r, err := os.Open(tarfile)
	assertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		assertNil(t, err)

		if header.Name == path {
			buf, err := ioutil.ReadAll(tr)
			assertNil(t, err)
			return true, string(buf)
		}
	}
	return false, ""
}

func assertTarFileOwner(t *testing.T, tarfile, path string, expectedUID, expectedGID int) {
	t.Helper()
	var foundPath bool
	r, err := os.Open(tarfile)
	assertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		assertNil(t, err)

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
