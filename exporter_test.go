package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	h "github.com/buildpack/lifecycle/testhelpers"
	"github.com/buildpack/lifecycle/testmock"
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
			UID: 1234,
			GID: 4321,
		}
	})

	it.After(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatal(err)
		}
	})

	when("#Export", func() {
		var (
			mockRunImage       *testmock.MockImage
			mockOrigImage      *testmock.MockImage
			mockController     *gomock.Controller
			appLayerSHA        string
			configLayerSHA     string
			buildpackLayer2SHA string
			buildpackLayer3SHA string
			layerSrc           string
			layersDst          string
			appSrc             string
			appDst             string
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockRunImage = testmock.NewMockImage(mockController)
			mockOrigImage = testmock.NewMockImage(mockController)

			layersDst = filepath.Join("/", "dest", "launch")

			appSrc = filepath.Join("testdata", "exporter", "first", "launch", "app")
			appDst = filepath.Join("/", "dest", "app")
			mockRunImage.EXPECT().TopLayer().Return("some-top-layer-sha", nil)
			mockRunImage.EXPECT().Digest().Return("some-run-image-digest", nil)
			mockOrigImage.EXPECT().Name().Return("app/repo")
			mockRunImage.EXPECT().Rename("app/repo")

			var err error
			layerSrc, err = ioutil.TempDir("", "lifecycle-layer-dir")
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
		})

		it.After(func() {
			os.RemoveAll(layerSrc)
			mockController.Finish()
		})

		when("previous image exists", func() {

			it.Before(func() {
				mockOrigImage.EXPECT().Found().Return(true, nil).AnyTimes()
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "first", "launch"), layerSrc)
			})

			it("creates the image on the registry", func() {
				mockOrigImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return(`{
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
          "sha": "sha256:3b62bb1034a4542c79ec6117baedbd4fb8948879a519c646c5528621ffa3d196"
        }
      }
    }
  ]
}
`, nil)
				mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
					t.Log("adds app layer")
					appLayerSHA = h.ComputeSHA256(t, layerPath)
					assertTarFileContents(t, layerPath, "/dest/app/.hidden.txt", "some-hidden-text\n")
					assertTarFileOwner(t, layerPath, "/dest/app", 1234, 4321)
					return nil
				})
				mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
					t.Log("adds config layer")
					configLayerSHA = h.ComputeSHA256(t, layerPath)
					assertTarFileContents(t,
						layerPath,
						"/dest/launch/config/metadata.toml",
						"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
					)
					assertTarFileOwner(t, layerPath, "/dest/launch/config", 1234, 4321)
					return nil
				})
				mockRunImage.EXPECT().ReuseLayer("orig-layer1-sha")
				mockRunImage.EXPECT().ReuseLayer("sha256:3b62bb1034a4542c79ec6117baedbd4fb8948879a519c646c5528621ffa3d196")
				mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
					t.Log("adds buildpack layer2")
					buildpackLayer2SHA = h.ComputeSHA256(t, layerPath)
					assertTarFileContents(t,
						layerPath,
						"/dest/launch/buildpack.id/layer2/file-from-layer-2",
						"echo text from layer 2\n")
					assertTarFileOwner(t, layerPath, "/dest/launch/buildpack.id/layer2", 1234, 4321)
					return nil
				})
				mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
					t.Log("adds buildpack layer3")
					buildpackLayer3SHA = h.ComputeSHA256(t, layerPath)
					assertTarFileContents(t,
						layerPath,
						"/dest/launch/other.buildpack.id/layer3/file-from-layer-3",
						"echo text from layer 3\n")
					assertTarFileOwner(t, layerPath, "/dest/launch/other.buildpack.id/layer3", 1234, 4321)
					return nil
				})

				mockRunImage.EXPECT().SetLabel("io.buildpacks.lifecycle.metadata", gomock.Any()).DoAndReturn(func(_, label string) error {
					t.Log("sets metadata label")
					var metadata lifecycle.AppImageMetadata
					if err := json.Unmarshal([]byte(label), &metadata); err != nil {
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
					return nil
				})
				mockRunImage.EXPECT().SetEnv("PACK_LAYERS_DIR", "/dest/launch")
				mockRunImage.EXPECT().SetEnv("PACK_APP_DIR", "/dest/app")
				mockRunImage.EXPECT().Save().Return("some-digest", nil)

				h.AssertNil(t, exporter.Export(layerSrc, layersDst, appSrc, appDst, mockRunImage, mockOrigImage))

				if !strings.Contains(stdout.String(), "Image: app/repo@some-digest") {
					t.Fatalf("output should contain Image: app/repo@some-digest, got '%s'", stdout.String())
				}
			})

			when("previous image metadata is missing buildpack for reused layer", func() {
				it.Before(func() {
					mockOrigImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
						Return(`{"buildpacks":[{}]}`, nil)
					mockRunImage.EXPECT().AddLayer(gomock.Any()).AnyTimes()
				})

				it("returns an error", func() {
					h.AssertError(
						t,
						exporter.Export(layerSrc, layersDst, appSrc, appDst, mockRunImage, mockOrigImage),
						"cannot reuse 'buildpack.id/layer1', previous image has no metadata for layer 'buildpack.id/layer1'",
					)
				})
			})

			when("previous image metadata is missing reused layer", func() {
				it.Before(func() {
					mockOrigImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
						Return(`{"buildpacks":[{"key": "buildpack.id", "layers": {}}]}`, nil)
					mockRunImage.EXPECT().AddLayer(gomock.Any()).AnyTimes()
				})

				it("returns an error", func() {
					h.AssertError(
						t,
						exporter.Export(layerSrc, layersDst, appSrc, appDst, mockRunImage, mockOrigImage),
						"cannot reuse 'buildpack.id/layer1', previous image has no metadata for layer 'buildpack.id/layer1'",
					)
				})
			})
		})

		when("previous image doesn't exist", func() {
			var buildpackLayer1SHA string

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "second", "launch"), layerSrc)
				mockOrigImage.EXPECT().Found().Return(false, nil).AnyTimes()
				mockOrigImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return("", errors.New("not exist")).AnyTimes()
			})

			it("makes new layers", func() {
				gomock.InOrder(
					mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
						t.Log("adds app layer")
						appLayerSHA = h.ComputeSHA256(t, layerPath)
						assertTarFileContents(t, layerPath, "/dest/app/.hidden.txt", "some-hidden-text\n")
						assertTarFileOwner(t, layerPath, "/dest/app", 1234, 4321)
						return nil
					}),
					mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
						t.Log("adds config layer")
						configLayerSHA = h.ComputeSHA256(t, layerPath)
						assertTarFileContents(t,
							layerPath,
							"/dest/launch/config/metadata.toml",
							"[[processes]]\n  type = \"web\"\n  command = \"npm start\"\n",
						)
						assertTarFileOwner(t, layerPath, "/dest/launch/config", 1234, 4321)
						return nil
					}),
					mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
						t.Log("adds buildpack layer1")
						buildpackLayer1SHA = h.ComputeSHA256(t, layerPath)
						assertTarFileContents(t,
							layerPath,
							"/dest/launch/buildpack.id/layer1/file-from-layer-1",
							"echo text from layer 1\n")
						assertTarFileOwner(t, layerPath, "/dest/launch/buildpack.id/layer1", 1234, 4321)
						return nil
					}),
					mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
						t.Log("adds buildpack layer2")
						buildpackLayer2SHA = h.ComputeSHA256(t, layerPath)
						assertTarFileContents(t,
							layerPath,
							"/dest/launch/buildpack.id/layer2/file-from-layer-2",
							"echo text from layer 2\n")
						assertTarFileOwner(t, layerPath, "/dest/launch/buildpack.id/layer2", 1234, 4321)
						return nil
					}),
				)

				mockRunImage.EXPECT().SetLabel("io.buildpacks.lifecycle.metadata", gomock.Any()).DoAndReturn(func(_, label string) error {
					t.Log("sets metadata label")
					var metadata lifecycle.AppImageMetadata
					if err := json.Unmarshal([]byte(label), &metadata); err != nil {
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
					return nil
				})
				mockRunImage.EXPECT().SetEnv("PACK_LAYERS_DIR", "/dest/launch")
				mockRunImage.EXPECT().SetEnv("PACK_APP_DIR", "/dest/app")
				mockRunImage.EXPECT().Save()

				h.AssertNil(t, exporter.Export(layerSrc, layersDst, appSrc, appDst, mockRunImage, mockOrigImage))
			})
		})

		when("dealing with cached layers", func() {
			var layer2sha string

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "exporter", "third", "launch"), layerSrc)
				mockOrigImage.EXPECT().Found().Return(true, nil).AnyTimes()
				mockOrigImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return(`{"buildpacks":[{"key": "buildpack.id", "layers": {"layer3": {"sha": "orig-layer3-sha"}}}]}`, nil)
				mockOrigImage.EXPECT().Found().Return(false, nil).AnyTimes()
				mockOrigImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return("", errors.New("not exist")).AnyTimes()

				mockRunImage.EXPECT().AddLayer(gomock.Any()).AnyTimes().Do(func(layerPath string) {
					buildpackLayer2SHA = h.ComputeSHA256(t, layerPath)
					if exist, _ := tarFileContext(t,
						layerPath,
						"/dest/launch/buildpack.id/layer2/file-from-layer-2"); exist {
						layer2sha = h.ComputeSHA256(t, layerPath)
					}
				})
				mockRunImage.EXPECT().ReuseLayer(gomock.Any()).AnyTimes()
				mockRunImage.EXPECT().SetLabel(gomock.Any(), gomock.Any()).AnyTimes()
				mockRunImage.EXPECT().SetEnv(gomock.Any(), gomock.Any()).AnyTimes()
				mockRunImage.EXPECT().Save()
			})

			it("deletes all non buildpack dirs", func() {
				h.AssertNil(t, exporter.Export(layerSrc, layersDst, appSrc, appDst, mockRunImage, mockOrigImage))

				if _, err := ioutil.ReadDir(filepath.Join(layerSrc, "app")); !os.IsNotExist(err) {
					t.Fatalf("Found app dir, it should not exist")
				}

				if _, err := ioutil.ReadDir(filepath.Join(layerSrc, "nonbuildpackdir")); !os.IsNotExist(err) {
					t.Fatalf("Found nonbuildpackdir dir, it should not exist")
				}

				if _, err := ioutil.ReadDir(filepath.Join(layerSrc, "config")); !os.IsNotExist(err) {
					t.Fatalf("Found config dir, it should not exist")
				}
			})

			it("deletes all uncached layers", func() {
				h.AssertNil(t, exporter.Export(layerSrc, layersDst, appSrc, appDst, mockRunImage, mockOrigImage))

				if _, err := ioutil.ReadDir(filepath.Join(layerSrc, "buildpack.id", "layer1")); !os.IsNotExist(err) {
					t.Fatalf("Found layer1 dir, it should not exist")
				}

				if _, err := ioutil.ReadDir(filepath.Join(layerSrc, "buildpack.id", "layer1.toml")); !os.IsNotExist(err) {
					t.Fatalf("Found layer1.toml, it should not exist")
				}
			})

			it("deletes layer.toml for all layers without a dir", func() {
				h.AssertNil(t, exporter.Export(layerSrc, layersDst, appSrc, appDst, mockRunImage, mockOrigImage))

				if _, err := ioutil.ReadDir(filepath.Join(layerSrc, "buildpack.id", "layer3.toml")); !os.IsNotExist(err) {
					t.Fatalf("Found layer3.toml, it should not exist")
				}
			})

			it("preserves cached layers and writes a sha", func() {
				h.AssertNil(t, exporter.Export(layerSrc, layersDst, appSrc, appDst, mockRunImage, mockOrigImage))

				if txt, err := ioutil.ReadFile(filepath.Join(layerSrc, "buildpack.id", "layer2", "file-from-layer-2")); err != nil || string(txt) != "echo text from layer 2\n" {
					t.Fatal("missing file-from-layer-2")
				}
				if _, err := ioutil.ReadFile(filepath.Join(layerSrc, "buildpack.id", "layer2.toml")); err != nil {
					t.Fatal("missing layer2.toml")
				}
				if txt, err := ioutil.ReadFile(filepath.Join(layerSrc, "buildpack.id", "layer2.sha")); err != nil {
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
		}
		if header.Uid != expectedUID {
			t.Fatalf("expected all entries in `%s` to have uid '%d', got '%d'", tarfile, expectedUID, header.Uid)
		}
		if header.Gid != expectedGID {
			t.Fatalf("expected all entries in `%s` to have gid '%d', got '%d'", tarfile, expectedGID, header.Gid)
		}
	}
	if !foundPath {
		t.Fatalf("%s does not exist in %s", path, tarfile)
	}
}
