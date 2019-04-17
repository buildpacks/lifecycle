package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/metadata"
	h "github.com/buildpack/lifecycle/testhelpers"
	"github.com/buildpack/lifecycle/testmock"
)

func TestExporter(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Exporter", testExporter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	var (
		exporter     *lifecycle.Exporter
		fakeRunImage *fakes.Image
		stderr       bytes.Buffer
		stdout       bytes.Buffer
		layersDir    string
		tmpDir       string
		appDir       string
		launcherPath string
		uid          = 1234
		gid          = 4321
		stack        = metadata.StackMetadata{}
	)

	it.Before(func() {
		stdout, stderr = bytes.Buffer{}, bytes.Buffer{}

		tmpDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
		h.AssertNil(t, err)

		launcherPath, err = filepath.Abs(filepath.Join("testdata", "exporter", "launcher"))
		h.AssertNil(t, err)

		layersDir = filepath.Join(tmpDir, "layers")
		h.AssertNil(t, os.Mkdir(layersDir, 0777))
		h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "launcher"), []byte("some-launcher"), 0777))

		fakeRunImage = fakes.NewImage(t, "runImageName", "some-top-layer-sha", "some-run-image-digest")

		exporter = &lifecycle.Exporter{
			ArtifactsDir: tmpDir,
			Buildpacks: []*lifecycle.Buildpack{
				{ID: "buildpack.id", Version: "1.2.3"},
				{ID: "other.buildpack.id", Version: "4.5.6"},
			},
			Out: log.New(&stdout, "", 0),
			Err: log.New(&stderr, "", 0),
			UID: uid,
			GID: gid,
		}
	})

	it.After(func() {
		fakeRunImage.Cleanup()

		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatal(err)
		}
	})

	when("#Export", func() {
		when("previous image exists", func() {
			var fakeOriginalImage *fakes.Image

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "previous-image-exists", "layers"), layersDir)

				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "previous-image-exists", "layers", "app"))
				h.AssertNil(t, err)

				localReusableLayerSha := h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/local-reusable-layer"), uid, gid)
				launcherSHA := h.ComputeSHA256ForPath(t, launcherPath, uid, gid)

				fakeOriginalImage = fakes.NewImage(t, "app/original-Image-Name", "original-top-layer-sha", "some-original-run-image-digest")
				_ = fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata",
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
              }`, localReusableLayerSha, launcherSHA))
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("creates app layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				appLayerPath := fakeRunImage.AppLayerPath()

				assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
				assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
				assertAddLayerLog(t, stdout, "app", appLayerPath)
			})

			it("creates config layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				configLayerPath := fakeRunImage.ConfigLayerPath()

				assertTarFileContents(t,
					configLayerPath,
					filepath.Join(layersDir, "config", "metadata.toml"),
					"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
				)
				assertTarFileOwner(t, configLayerPath, filepath.Join(layersDir, "config"), uid, gid)
				assertAddLayerLog(t, stdout, "config", configLayerPath)
			})

			it("reuses launcher layer if the sha matches the sha in the metadata", func() {
				launcherLayerSHA := h.ComputeSHA256ForPath(t, launcherPath, uid, gid)
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))
				h.AssertContains(t, fakeRunImage.ReusedLayers(), "sha256:"+launcherLayerSHA)
				assertReuseLayerLog(t, stdout, "launcher", launcherLayerSHA)
			})

			it("reuses launch layers when only layer.toml is present", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				h.AssertContains(t, fakeRunImage.ReusedLayers(), "sha256:orig-launch-layer-no-local-dir-sha")
				assertReuseLayerLog(t, stdout, "buildpack.id:launch-layer-no-local-dir", "orig-launch-layer-no-local-dir-sha")
			})

			it("reuses cached launch layers if the local sha matches the sha in the metadata", func() {
				layer5sha := h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/local-reusable-layer"), uid, gid)

				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				h.AssertContains(t, fakeRunImage.ReusedLayers(), "sha256:"+layer5sha)
				assertReuseLayerLog(t, stdout, "other.buildpack.id:local-reusable-layer", layer5sha)
			})

			it("adds new launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				layer2Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/new-launch-layer"))

				assertTarFileContents(t,
					layer2Path,
					filepath.Join(layersDir, "buildpack.id/new-launch-layer/file-from-new-launch-layer"),
					"echo text from layer 2\n")
				assertTarFileOwner(t, layer2Path, filepath.Join(layersDir, "buildpack.id/new-launch-layer"), uid, gid)
				assertAddLayerLog(t, stdout, "buildpack.id:new-launch-layer", layer2Path)
			})

			it("adds new launch layers from a second buildpack", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				layer3Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"))

				assertTarFileContents(t,
					layer3Path,
					filepath.Join(layersDir, "other.buildpack.id/new-launch-layer/new-launch-layer-file"),
					"echo text from new layer\n")
				assertTarFileOwner(t, layer3Path, filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"), uid, gid)
				assertAddLayerLog(t, stdout, "other.buildpack.id:new-launch-layer", layer3Path)
			})

			it("only creates expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				var applayer, configLayer, layer2, layer3 = 1, 1, 1, 1
				h.AssertEq(t, fakeRunImage.NumberOfAddedLayers(), applayer+configLayer+layer2+layer3)
			})

			it("only reuses expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				var launcherLayer, layer1, layer5 = 1, 1, 1
				h.AssertEq(t, len(fakeRunImage.ReusedLayers()), launcherLayer+layer1+layer5)
			})

			it("saves lifecycle metadata with layer info", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				appLayerPath := fakeRunImage.AppLayerPath()
				appLayerSHA := h.ComputeSHA256ForFile(t, appLayerPath)

				configLayerPath := fakeRunImage.ConfigLayerPath()
				configLayerSHA := h.ComputeSHA256ForFile(t, configLayerPath)

				newLayerPath := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/new-launch-layer"))
				newLayerSHA := h.ComputeSHA256ForFile(t, newLayerPath)

				secondBPLayerPath := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "other.buildpack.id/new-launch-layer"))
				secondBPLayerPathSHA := h.ComputeSHA256ForFile(t, secondBPLayerPath)

				metadataJSON, err := fakeRunImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta metadata.AppImageMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				t.Log("adds run image metadata to label")
				h.AssertEq(t, meta.RunImage.TopLayer, "some-top-layer-sha")
				h.AssertEq(t, meta.RunImage.SHA, "some-run-image-digest")

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
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				metadataJSON, err := fakeRunImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta metadata.AppImageMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}
				h.AssertNil(t, err)
				h.AssertEq(t, meta.Stack.RunImage.Image, "some/run")
				h.AssertEq(t, meta.Stack.RunImage.Mirrors, []string{"registry.example.com/some/run", "other.example.com/some/run"})
			})

			it("sets CNB_LAYERS_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				val, err := fakeRunImage.Env("CNB_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, layersDir)
			})

			it("sets CNB_APP_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				val, err := fakeRunImage.Env("CNB_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, appDir)
			})

			it("sets ENTRYPOINT to launcher", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				val, err := fakeRunImage.Entrypoint()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string{launcherPath})
			})

			it("sets empty CMD", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				val, err := fakeRunImage.Cmd()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string(nil))
			})

			it("sets name to match old run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				h.AssertEq(t, fakeRunImage.Name(), "app/original-Image-Name")
			})

			it("saves run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				h.AssertEq(t, fakeRunImage.IsSaved(), true)
			})

			it("outputs image name and digest", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				if !strings.Contains(stdout.String(), "Image: app/original-Image-Name@saved-digest-from-fake-run-image") {
					t.Fatalf("output should contain Image: app/original-Image-Name@some-digest, got '%s'", stdout.String())
				}
			})

			when("previous image metadata is missing buildpack for reused layer", func() {
				it.Before(func() {
					_ = fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata", `{"buildpacks":[{}]}`)
				})

				it("returns an error", func() {
					h.AssertError(
						t,
						exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack),
						"cannot reuse 'buildpack.id:launch-layer-no-local-dir', previous image has no metadata for layer 'buildpack.id:launch-layer-no-local-dir'",
					)
				})
			})

			when("previous image metadata is missing reused layer", func() {
				it.Before(func() {
					_ = fakeOriginalImage.SetLabel(
						"io.buildpacks.lifecycle.metadata",
						`{"buildpacks":[{"key": "buildpack.id", "layers": {}}]}`)
				})

				it("returns an error", func() {
					h.AssertError(
						t,
						exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack),
						"cannot reuse 'buildpack.id:launch-layer-no-local-dir', previous image has no metadata for layer 'buildpack.id:launch-layer-no-local-dir'",
					)
				})
			})
		})

		when("previous image doesn't exist", func() {
			var (
				nonExistingOriginalImage imgutil.Image
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "previous-image-not-exist", "layers"), layersDir)
				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "previous-image-not-exist", "layers", "app"))
				h.AssertNil(t, err)

				mockNonExistingOriginalImage := testmock.NewMockImage(gomock.NewController(t))

				mockNonExistingOriginalImage.EXPECT().Name().Return("app/original-Image-Name").AnyTimes()
				mockNonExistingOriginalImage.EXPECT().Found().Return(false, nil)
				mockNonExistingOriginalImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return("", errors.New("not exist")).AnyTimes()

				nonExistingOriginalImage = mockNonExistingOriginalImage
			})

			it("creates app layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				appLayerPath := fakeRunImage.AppLayerPath()

				assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
				assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
				assertAddLayerLog(t, stdout, "app", appLayerPath)
			})

			it("creates config layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				configLayerPath := fakeRunImage.ConfigLayerPath()

				assertTarFileContents(t,
					configLayerPath,
					filepath.Join(layersDir, "config/metadata.toml"),
					"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
				)
				assertTarFileOwner(t, configLayerPath, filepath.Join(layersDir, "config"), uid, gid)
				assertAddLayerLog(t, stdout, "config", configLayerPath)
			})

			it("creates a launcher layer", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				launcherLayerPath := fakeRunImage.FindLayerWithPath(launcherPath)
				assertTarFileContents(t,
					launcherLayerPath,
					launcherPath,
					"some-launcher")
				assertTarFileOwner(t, launcherLayerPath, launcherPath, uid, gid)
				assertAddLayerLog(t, stdout, "launcher", launcherLayerPath)
			})

			it("adds launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				layer1Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer1"))
				assertTarFileContents(t,
					layer1Path,
					filepath.Join(layersDir, "buildpack.id/layer1/file-from-layer-1"),
					"echo text from layer 1\n")
				assertTarFileOwner(t, layer1Path, filepath.Join(layersDir, "buildpack.id/layer1"), uid, gid)
				assertAddLayerLog(t, stdout, "buildpack.id:layer1", layer1Path)

				layer2Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				assertTarFileContents(t,
					layer2Path,
					filepath.Join(layersDir, "buildpack.id/layer2/file-from-layer-2"),
					"echo text from layer 2\n")
				assertTarFileOwner(t, layer2Path, filepath.Join(layersDir, "buildpack.id/layer2"), uid, gid)
				assertAddLayerLog(t, stdout, "buildpack.id:layer2", layer2Path)
			})

			it("only creates expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				var applayer, configLayer, launcherLayer, layer1, layer2 = 1, 1, 1, 1, 1
				h.AssertEq(t, fakeRunImage.NumberOfAddedLayers(), applayer+configLayer+launcherLayer+layer1+layer2)
			})

			it("saves metadata with layer info", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				appLayerPath := fakeRunImage.AppLayerPath()
				appLayerSHA := h.ComputeSHA256ForFile(t, appLayerPath)

				configLayerPath := fakeRunImage.ConfigLayerPath()
				configLayerSHA := h.ComputeSHA256ForFile(t, configLayerPath)

				launcherLayerPath := fakeRunImage.FindLayerWithPath(launcherPath)
				launcherLayerSHA := h.ComputeSHA256ForFile(t, launcherLayerPath)

				layer1Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer1"))
				buildpackLayer1SHA := h.ComputeSHA256ForFile(t, layer1Path)

				layer2Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				buildpackLayer2SHA := h.ComputeSHA256ForFile(t, layer2Path)

				metadataJSON, err := fakeRunImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta metadata.AppImageMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				t.Log("adds run image metadata to label")
				h.AssertEq(t, meta.RunImage.TopLayer, "some-top-layer-sha")
				h.AssertEq(t, meta.RunImage.SHA, "some-run-image-digest")

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
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				val, err := fakeRunImage.Env("CNB_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, layersDir)
			})

			it("sets CNB_APP_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				val, err := fakeRunImage.Env("CNB_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, appDir)
			})

			it("sets ENTRYPOINT to launcher", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				val, err := fakeRunImage.Entrypoint()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string{launcherPath})
			})

			it("sets empty CMD", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				val, err := fakeRunImage.Cmd()
				h.AssertNil(t, err)
				h.AssertEq(t, val, []string(nil))
			})

			it("sets name to match original image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				h.AssertEq(t, fakeRunImage.Name(), "app/original-Image-Name")
			})

			it("saves run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage, launcherPath, stack))

				h.AssertEq(t, fakeRunImage.IsSaved(), true)
			})
		})

		when("buildpack requires an escaped id", func() {
			var (
				fakeOriginalImage *fakes.Image
			)

			it.Before(func() {
				exporter.Buildpacks = []*lifecycle.Buildpack{
					{ID: "some/escaped/bp/id"},
				}

				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "escaped-bpid", "layers"), layersDir)

				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "escaped-bpid", "layers", "app"))
				h.AssertNil(t, err)

				fakeOriginalImage = fakes.NewImage(t, "app/original", "original-top-sha", "run-digest")
				fakeOriginalImage.SetLabel(
					"io.buildpacks.lifecycle.metadata",
					`{"buildpacks":[{"key": "some/escaped/bp/id", "layers": {"layer": {"sha": "original-layer-sha"}}}]}`,
				)
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("exports layers from the escaped id path", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				layerPath := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "some_escaped_bp_id/some-layer"))

				assertTarFileContents(t,
					layerPath,
					filepath.Join(layersDir, "some_escaped_bp_id/some-layer/some-file"),
					"some-file-contents\n")
				assertTarFileOwner(t, layerPath, filepath.Join(layersDir, "some_escaped_bp_id/some-layer/some-file"), uid, gid)
				assertAddLayerLog(t, stdout, "some/escaped/bp/id:some-layer", layerPath)
			})

			it("exports buildpack metadata with unescaped id", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack))

				metadataJSON, err := fakeRunImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var meta metadata.AppImageMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				h.AssertEq(t, meta.Buildpacks[0].ID, "some/escaped/bp/id")
				h.AssertEq(t, len(meta.Buildpacks[0].Layers), 1)
			})
		})

		when("there is an invalid layer.toml", func() {
			var (
				mockNonExistingOriginalImage *testmock.MockImage
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "bad-layer", "layers"), layersDir)
				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "bad-layer", "layers", "app"))
				h.AssertNil(t, err)

				mockNonExistingOriginalImage = testmock.NewMockImage(gomock.NewController(t))
				mockNonExistingOriginalImage.EXPECT().Name().Return("app/original-Image-Name").AnyTimes()
				mockNonExistingOriginalImage.EXPECT().Found().Return(false, nil)
				mockNonExistingOriginalImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return("", errors.New("not exist")).AnyTimes()
			})

			it("returns an error", func() {
				h.AssertError(
					t,
					exporter.Export(layersDir, appDir, fakeRunImage, mockNonExistingOriginalImage, launcherPath, stack),
					"failed to parse metadata for layers '[buildpack.id:bad-layer]'",
				)
			})
		})

		when("there is a launch=true cache=true layer without contents", func() {
			var (
				fakeOriginalImage *fakes.Image
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "cache-layer-no-contents", "layers"), layersDir)
				var err error
				appDir, err = filepath.Abs(filepath.Join("testdata", "exporter", "cache-layer-no-contents", "layers", "app"))
				h.AssertNil(t, err)

				fakeOriginalImage = fakes.NewImage(t, "app/original-Image-Name", "original-top-layer-sha", "some-original-run-image-digest")
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
			})

			it.After(func() {
				fakeOriginalImage.Cleanup()
			})

			it("returns an error", func() {
				h.AssertError(
					t,
					exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage, launcherPath, stack),
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
	if !strings.Contains(stdout.String(), expected) {
		t.Fatalf("Expected output \n'%s' to contain \n'%s'", stdout.String(), expected)
	}
}

func assertReuseLayerLog(t *testing.T, stdout bytes.Buffer, name, sha string) {
	t.Helper()
	expected := fmt.Sprintf("Reusing layer '%s' with SHA sha256:%s", name, sha)
	if !strings.Contains(stdout.String(), expected) {
		t.Fatalf("Expected output \n\"%s\"\n to contain \n\"%s\"", stdout.String(), expected)
	}
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
