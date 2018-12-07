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
			launchSrc          string
			launchDst          string
			appSrc             string
			appDst             string
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockRunImage = testmock.NewMockImage(mockController)
			mockOrigImage = testmock.NewMockImage(mockController)
			launchSrc = filepath.Join("testdata", "exporter", "first", "launch")
			launchDst = filepath.Join("/", "dest", "launch")
			appSrc = filepath.Join("testdata", "exporter", "first", "launch", "app")
			appDst = filepath.Join("/", "dest", "app")
			mockRunImage.EXPECT().TopLayer().Return("some-top-layer-sha", nil)
			mockRunImage.EXPECT().Digest().Return("some-run-image-digest", nil)
			mockOrigImage.EXPECT().Name().Return("app/repo")
			mockRunImage.EXPECT().Rename("app/repo")
		})

		it.After(func() {
			mockController.Finish()
		})

		when("previous image exists", func() {

			it.Before(func() {
				mockOrigImage.EXPECT().Found().Return(true, nil).AnyTimes()
			})

			it("creates the image on the registry", func() {
				mockOrigImage.EXPECT().Label("io.buildpacks.lifecycle.metadata").
					Return(`{"buildpacks":[{"key": "buildpack.id", "layers": {"layer1": {"sha": "orig-layer1-sha", "data": {"oldkey":"oldval"}}}}]}`, nil)
				mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
					t.Log("adds app layer")
					appLayerSHA = h.ComputeSHA256(t, layerPath)
					assertTarFileContents(t, layerPath, "/dest/app/.hidden.txt", "some-hidden-text\n")
					assertTarFileOwner(t, layerPath, "/dest/app/", 1234, 4321)
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
					assertTarFileOwner(t, layerPath, "/dest/launch/config/", 1234, 4321)
					return nil
				})
				mockRunImage.EXPECT().ReuseLayer("orig-layer1-sha")
				mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
					t.Log("adds buildpack layer2")
					buildpackLayer2SHA = h.ComputeSHA256(t, layerPath)
					assertTarFileContents(t,
						layerPath,
						"/dest/launch/buildpack.id/layer2/file-from-layer-2",
						"echo text from layer 2\n")
					assertTarFileOwner(t, layerPath, "/dest/launch/buildpack.id/layer2/", 1234, 4321)
					return nil
				})
				mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
					t.Log("adds buildpack layer3")
					buildpackLayer3SHA = h.ComputeSHA256(t, layerPath)
					assertTarFileContents(t,
						layerPath,
						"/dest/launch/other.buildpack.id/layer3/file-from-layer-3",
						"echo text from layer 3\n")
					assertTarFileOwner(t, layerPath, "/dest/launch/other.buildpack.id/layer3/", 1234, 4321)
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

				h.AssertNil(t, exporter.Export(launchSrc, launchDst, appSrc, appDst, mockRunImage, mockOrigImage))

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
						exporter.Export(launchSrc, launchDst, appSrc, appDst, mockRunImage, mockOrigImage),
						"cannot reuse 'buildpack.id/layer1', previous image has no metadata for buildpack 'buildpack.id'",
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
						exporter.Export(launchSrc, launchDst, appSrc, appDst, mockRunImage, mockOrigImage),
						"cannot reuse 'buildpack.id/layer1', previous image has no metadata for layer 'buildpack.id/layer1'",
					)
				})
			})
		})

		when("previous image doesn't exist", func() {
			var buildpackLayer1SHA string

			it.Before(func() {
				launchSrc = filepath.Join("testdata", "exporter", "second", "launch")
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
						assertTarFileOwner(t, layerPath, "/dest/app/", 1234, 4321)
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
						assertTarFileOwner(t, layerPath, "/dest/launch/config/", 1234, 4321)
						return nil
					}),
					mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
						t.Log("adds buildpack layer1")
						buildpackLayer1SHA = h.ComputeSHA256(t, layerPath)
						assertTarFileContents(t,
							layerPath,
							"/dest/launch/buildpack.id/layer1/file-from-layer-1",
							"echo text from layer 1\n")
						assertTarFileOwner(t, layerPath, "/dest/launch/buildpack.id/layer1/", 1234, 4321)
						return nil
					}),
					mockRunImage.EXPECT().AddLayer(gomock.Any()).DoAndReturn(func(layerPath string) error {
						t.Log("adds buildpack layer2")
						buildpackLayer2SHA = h.ComputeSHA256(t, layerPath)
						assertTarFileContents(t,
							layerPath,
							"/dest/launch/buildpack.id/layer2/file-from-layer-2",
							"echo text from layer 2\n")
						assertTarFileOwner(t, layerPath, "/dest/launch/buildpack.id/layer2/", 1234, 4321)
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

				h.AssertNil(t, exporter.Export(launchSrc, launchDst, appSrc, appDst, mockRunImage, mockOrigImage))
			})
		})
	})
}

func assertTarFileContents(t *testing.T, tarfile, path, expected string) {
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
			assertEq(t, string(buf), expected)
			return
		}
	}
	t.Fatalf("%s does not exist in %s", path, tarfile)
}

func assertTarFileOwner(t *testing.T, tarfile, path string, expectedUID, expectedGID int) {
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
