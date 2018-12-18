package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/buildpack/lifecycle/image"
	"github.com/buildpack/lifecycle/testmock"
	"github.com/golang/mock/gomock"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestExporter(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Exporter", testExporter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	var (
		exporter *lifecycle.Exporter
		stderr   bytes.Buffer
		stdout   bytes.Buffer
		tmpDir   string
		uid      = 1234
		gid      = 4321
	)

	it.Before(func() {
		stdout, stderr = bytes.Buffer{}, bytes.Buffer{}
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle.exporter.layer")
		if err != nil {
			t.Fatal(err)
		}
		exporter = &lifecycle.Exporter{
			ArtifactsDir: tmpDir,
			Buildpacks: []*lifecycle.Buildpack{
				{ID: "buildpack.id"},
				{ID: "other.buildpack.id"},
			},
			Out: log.New(&stdout, "", 0),
			Err: log.New(&stderr, "", 0),
			UID: uid,
			GID: gid,
		}
	})

	it.After(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatal(err)
		}
	})

	when("#Export", func() {
		var (
			appLayerSHA        string
			configLayerSHA     string
			buildpackLayer2SHA string
			buildpackLayer3SHA string
			layersDir          string
			appDir             string
			fakeRunImage       *h.FakeImage
			fakeOriginalImage  *h.FakeImage
		)

		it.Before(func() {
			appDir = filepath.Join("testdata", "exporter", "first", "launch", "app")

			fakeOriginalImage = h.NewFakeImage(t, "app/original-Image-Name", "original-top-layer-sha", "some-original-run-image-digest")
			fakeRunImage = h.NewFakeImage(t, "runImageName", "some-top-layer-sha", "some-run-image-digest")

			var err error
			layersDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
		})

		it.After(func() {
			os.RemoveAll(layersDir)
		})

		when("previous image exists", func() {

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "first", "launch"), layersDir)

				layer5sha := h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/layer5"), uid, gid)

				_ = fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata",
					fmt.Sprintf(`{
				  "buildpacks": [
				    {
				      "key": "buildpack.id",
				      "layers": {
				        "layer1": {
				          "sha": "orig-layer1-sha",
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
				        "layer5": {
				          "sha": "sha256:%s"
				        }
				      }
				    }
				  ]
				}
				`, layer5sha))

			})

			it("creates app layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				appLayerPath := fakeRunImage.AppLayerPath()

				assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
				assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
			})

			it("creates config layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				configLayerPath := fakeRunImage.ConfigLayerPath()

				assertTarFileContents(t,
					configLayerPath,
					filepath.Join(layersDir, "config", "metadata.toml"),
					"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
				)
				assertTarFileOwner(t, configLayerPath, filepath.Join(layersDir, "config"), uid, gid)
			})

			it("resuses cached launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				h.AssertContains(t, fakeRunImage.ReusedLayers(), "orig-layer1-sha")
			})

			it("resuses launch layers if the local sha matches the sha in the metadata", func() {
				layer5sha := h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/layer5"), uid, gid)

				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				h.AssertContains(t, fakeRunImage.ReusedLayers(), "sha256:"+layer5sha)
			})

			it("adds new launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				layer2Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))

				assertTarFileContents(t,
					layer2Path,
					filepath.Join(layersDir, "buildpack.id/layer2/file-from-layer-2"),
					"echo text from layer 2\n")
				assertTarFileOwner(t, layer2Path, filepath.Join(layersDir, "buildpack.id/layer2"), uid, gid)
			})

			it("adds new launch layers from a second buildpack", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				layer3Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "other.buildpack.id/layer3"))

				assertTarFileContents(t,
					layer3Path,
					filepath.Join(layersDir, "other.buildpack.id/layer3/file-from-layer-3"),
					"echo text from layer 3\n")
				assertTarFileOwner(t, layer3Path, filepath.Join(layersDir, "other.buildpack.id/layer3"), uid, gid)
			})

			it("only creates expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				var applayer, configLayer, layer2, layer3 = 1, 1, 1, 1
				h.AssertEq(t, fakeRunImage.NumberOfLayers(), applayer+configLayer+layer2+layer3)
			})

			it("only reuses expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				var layer1, layer5 = 1, 1
				h.AssertEq(t, len(fakeRunImage.ReusedLayers()), layer1+layer5)
			})

			it("saves metadata with layer info", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				appLayerPath := fakeRunImage.AppLayerPath()
				appLayerSHA = h.ComputeSHA256ForFile(t, appLayerPath)

				configLayerPath := fakeRunImage.ConfigLayerPath()
				configLayerSHA = h.ComputeSHA256ForFile(t, configLayerPath)

				layer2Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				buildpackLayer2SHA = h.ComputeSHA256ForFile(t, layer2Path)

				layer3Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "other.buildpack.id/layer3"))
				buildpackLayer3SHA = h.ComputeSHA256ForFile(t, layer3Path)

				metadataJSON, err := fakeRunImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var metadata lifecycle.AppImageMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				t.Log("adds run image metadata to label")
				h.AssertEq(t, metadata.RunImage.TopLayer, "some-top-layer-sha")
				h.AssertEq(t, metadata.RunImage.SHA, "some-run-image-digest")

				t.Log("adds layer shas to metadata label")
				h.AssertEq(t, metadata.App.SHA, "sha256:"+appLayerSHA)
				h.AssertEq(t, metadata.Config.SHA, "sha256:"+configLayerSHA)
				h.AssertEq(t, metadata.Buildpacks[0].Layers["layer1"].SHA, "orig-layer1-sha")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["layer2"].SHA, "sha256:"+buildpackLayer2SHA)
				h.AssertEq(t, metadata.Buildpacks[1].Layers["layer3"].SHA, "sha256:"+buildpackLayer3SHA)

				t.Log("adds buildpack layer metadata to label")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["layer1"].Data, map[string]interface{}{
					"oldkey": "oldval",
				})
				h.AssertEq(t, metadata.Buildpacks[0].Layers["layer2"].Data, map[string]interface{}{
					"somekey": "someval",
				})
			})

			it("sets PACK_LAYERS_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				val, err := fakeRunImage.Env("PACK_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, layersDir)
			})

			it("sets PACK_APP_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				val, err := fakeRunImage.Env("PACK_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, appDir)
			})

			it("sets name to match old run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				h.AssertEq(t, fakeRunImage.Name(), "app/original-Image-Name")
			})

			it("saves run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				h.AssertEq(t, fakeRunImage.IsSaved(), true)
			})

			it("outputs image name and digest", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

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
						exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage),
						"cannot reuse 'buildpack.id/layer1', previous image has no metadata for layer 'buildpack.id/layer1'",
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
						exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage),
						"cannot reuse 'buildpack.id/layer1', previous image has no metadata for layer 'buildpack.id/layer1'",
					)
				})
			})
		})

		when("previous image doesn't exist", func() {
			var (
				buildpackLayer1SHA       string
				nonExistingOriginalImage image.Image
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "second", "launch"), layersDir)

				mockNonExistingOriginalImage := testmock.NewMockImage(gomock.NewController(t))

				mockNonExistingOriginalImage.EXPECT().Name().Return("app/original-Image-Name")
				mockNonExistingOriginalImage.EXPECT().Found().Return(false, nil)
				mockNonExistingOriginalImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return("", errors.New("not exist")).AnyTimes()

				nonExistingOriginalImage = mockNonExistingOriginalImage
			})

			it("creates app layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				appLayerPath := fakeRunImage.AppLayerPath()

				assertTarFileContents(t, appLayerPath, filepath.Join(appDir, ".hidden.txt"), "some-hidden-text\n")
				assertTarFileOwner(t, appLayerPath, appDir, uid, gid)
			})

			it("creates config layer on Run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				configLayerPath := fakeRunImage.ConfigLayerPath()

				assertTarFileContents(t,
					configLayerPath,
					filepath.Join(layersDir, "config/metadata.toml"),
					"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
				)
				assertTarFileOwner(t, configLayerPath, filepath.Join(layersDir, "config"), uid, gid)
			})

			it("adds launch layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				layer1Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer1"))
				assertTarFileContents(t,
					layer1Path,
					filepath.Join(layersDir, "buildpack.id/layer1/file-from-layer-1"),
					"echo text from layer 1\n")
				assertTarFileOwner(t, layer1Path, filepath.Join(layersDir, "buildpack.id/layer1"), uid, gid)

				layer2Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				assertTarFileContents(t,
					layer2Path,
					filepath.Join(layersDir, "buildpack.id/layer2/file-from-layer-2"),
					"echo text from layer 2\n")
				assertTarFileOwner(t, layer2Path, filepath.Join(layersDir, "buildpack.id/layer2"), uid, gid)
			})

			it("only creates expected layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				var applayer, configLayer, layer1, layer2 = 1, 1, 1, 1
				h.AssertEq(t, fakeRunImage.NumberOfLayers(), applayer+configLayer+layer1+layer2)
			})

			it("saves metadata with layer info", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				appLayerPath := fakeRunImage.AppLayerPath()
				appLayerSHA = h.ComputeSHA256ForFile(t, appLayerPath)

				configLayerPath := fakeRunImage.ConfigLayerPath()
				configLayerSHA = h.ComputeSHA256ForFile(t, configLayerPath)

				layer2Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer1"))
				buildpackLayer1SHA = h.ComputeSHA256ForFile(t, layer2Path)

				layer3Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				buildpackLayer2SHA = h.ComputeSHA256ForFile(t, layer3Path)

				metadataJSON, err := fakeRunImage.Label("io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, err)

				var metadata lifecycle.AppImageMetadata
				if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				t.Log("adds run image metadata to label")
				h.AssertEq(t, metadata.RunImage.TopLayer, "some-top-layer-sha")
				h.AssertEq(t, metadata.RunImage.SHA, "some-run-image-digest")

				t.Log("adds layer shas to metadata label")
				h.AssertEq(t, metadata.App.SHA, "sha256:"+appLayerSHA)
				h.AssertEq(t, metadata.Config.SHA, "sha256:"+configLayerSHA)
				h.AssertEq(t, metadata.Buildpacks[0].Layers["layer1"].SHA, "sha256:"+buildpackLayer1SHA)
				h.AssertEq(t, metadata.Buildpacks[0].Layers["layer2"].SHA, "sha256:"+buildpackLayer2SHA)

				t.Log("adds buildpack layer metadata to label")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["layer1"].Data, map[string]interface{}{
					"mykey": "new val",
				})
			})

			it("sets PACK_LAYERS_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				val, err := fakeRunImage.Env("PACK_LAYERS_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, layersDir)
			})

			it("sets PACK_APP_DIR", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				val, err := fakeRunImage.Env("PACK_APP_DIR")
				h.AssertNil(t, err)
				h.AssertEq(t, val, appDir)
			})

			it("sets name to match original image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				h.AssertEq(t, fakeRunImage.Name(), "app/original-Image-Name")
			})

			it("saves run image", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, nonExistingOriginalImage))

				h.AssertEq(t, fakeRunImage.IsSaved(), true)
			})
		})

		when("dealing with cached layers", func() {
			var layer2sha string

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "third", "launch"), layersDir)

				_ = fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.metadata",
					`{"buildpacks":[{"key": "buildpack.id", "layers": {"layer3": {"sha": "orig-layer3-sha"}}}]}`)
			})

			it("deletes all non buildpack dirs", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				if _, err := ioutil.ReadDir(filepath.Join(layersDir, "app")); !os.IsNotExist(err) {
					t.Fatalf("Found app dir, it should not exist")
				}

				if _, err := ioutil.ReadDir(filepath.Join(layersDir, "nonbuildpackdir")); !os.IsNotExist(err) {
					t.Fatalf("Found nonbuildpackdir dir, it should not exist")
				}

				if _, err := ioutil.ReadDir(filepath.Join(layersDir, "config")); !os.IsNotExist(err) {
					t.Fatalf("Found config dir, it should not exist")
				}
			})

			it("deletes all uncached layers", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				if _, err := ioutil.ReadDir(filepath.Join(layersDir, "buildpack.id", "layer1")); !os.IsNotExist(err) {
					t.Fatalf("Found layer1 dir, it should not exist")
				}

				if _, err := ioutil.ReadDir(filepath.Join(layersDir, "buildpack.id", "layer1.toml")); !os.IsNotExist(err) {
					t.Fatalf("Found layer1.toml, it should not exist")
				}
			})

			it("deletes layer.toml for all layers without a dir", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				if _, err := ioutil.ReadDir(filepath.Join(layersDir, "buildpack.id", "layer3.toml")); !os.IsNotExist(err) {
					t.Fatalf("Found layer3.toml, it should not exist")
				}
			})

			it("preserves cached layers and writes a sha", func() {
				h.AssertNil(t, exporter.Export(layersDir, appDir, fakeRunImage, fakeOriginalImage))

				layer2Path := fakeRunImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/layer2"))
				layer2sha = h.ComputeSHA256ForFile(t, layer2Path)

				if txt, err := ioutil.ReadFile(filepath.Join(layersDir, "buildpack.id", "layer2", "file-from-layer-2")); err != nil || string(txt) != "echo text from layer 2\n" {
					t.Fatal("missing file-from-layer-2")
				}
				if _, err := ioutil.ReadFile(filepath.Join(layersDir, "buildpack.id", "layer2.toml")); err != nil {
					t.Fatal("missing layer2.toml")
				}
				if txt, err := ioutil.ReadFile(filepath.Join(layersDir, "buildpack.id", "layer2.sha")); err != nil {
					t.Fatal("missing layer2.sha")
				} else if string(txt) != "sha256:"+layer2sha {
					t.Fatalf("expected layer.sha to have sha '%s', got '%s'", layer2sha, string(txt))
				}
			})
		})
	})
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
