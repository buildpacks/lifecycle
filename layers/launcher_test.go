package layers_test

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLauncherLayers(t *testing.T) {
	spec.Run(t, "Factory", testLauncherLayers, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLauncherLayers(t *testing.T, when spec.G, it spec.S) {
	var (
		factory    *layers.Factory
		logHandler = memory.New()
	)
	it.Before(func() {
		var err error
		artifactDir, err := ioutil.TempDir("", "layers.slices.layer")
		h.AssertNil(t, err)
		factory = &layers.Factory{
			ArtifactsDir: artifactDir,
			Logger:       &log.Logger{Handler: logHandler},
			UID:          1234,
			GID:          4321,
		}
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(factory.ArtifactsDir))
	})

	when("#ProcessTypesLayer", func() {
		it("creates a layer containing the config file and process type symlinks", func() {
			proc1 := launch.Process{Type: "some-type"}
			proc2 := launch.Process{Type: "other-type"}
			configLayer, err := factory.ProcessTypesLayer(launch.Metadata{Processes: []launch.Process{
				proc1,
				proc2,
			}})
			h.AssertNil(t, err)
			h.AssertEq(t, configLayer.ID, "process-types")
			assertTarEntries(t, configLayer.TarPath, []*tar.Header{
				{
					Name:     tarPath("/cnb"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath("/cnb/process"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath(launch.ProcessPath(proc1.Type)),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeSymlink,
					Linkname: launch.LauncherPath,
				},
				{
					Name:     tarPath(launch.ProcessPath(proc2.Type)),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeSymlink,
					Linkname: launch.LauncherPath,
				},
			})
		})

		when("process-type contains invalid character", func() {
			it("returns an error", func() {
				_, err := factory.ProcessTypesLayer(launch.Metadata{Processes: []launch.Process{
					{Type: "bad>"},
				}})
				h.AssertError(t, err, "invalid process type 'bad>'")
			})
		})

		when("process-type is empty", func() {
			it("returns an error", func() {
				_, err := factory.ProcessTypesLayer(launch.Metadata{Processes: []launch.Process{
					{Type: ""},
				}})
				h.AssertError(t, err, "type is required for all processes")
			})
		})
	})

	when("#LauncherLayer", func() {
		it("creates a layer with the launcher", func() {
			launcherLayer, err := factory.LauncherLayer(filepath.Join("testdata", "fake-launcher"))
			h.AssertNil(t, err)
			h.AssertEq(t, launcherLayer.ID, "launcher")
			assertTarEntries(t, launcherLayer.TarPath, []*tar.Header{
				{
					Name:     tarPath("/cnb"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath("/cnb/lifecycle"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath(launch.LauncherPath),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeReg,
					Linkname: launch.LauncherPath,
				},
			})
			assertEntryContent(t, launcherLayer.TarPath, tarPath(launch.LauncherPath), "launcher-content")
		})
	})
}

func assertEntryContent(t *testing.T, tarPath string, name string, expected string) {
	t.Helper()
	lf, err := os.Open(tarPath)
	h.AssertNil(t, err)
	defer lf.Close()
	tr := tar.NewReader(lf)

	var allEntryNames []string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("missing expected archive entry '%s'\n archive contained %v", name, allEntryNames)
		}
		h.AssertNil(t, err)
		allEntryNames = append(allEntryNames, header.Name)
		if header.Name != name {
			continue
		}
		content := make([]byte, header.Size)
		_, err = tr.Read(content)
		h.AssertSameInstance(t, err, io.EOF)
		h.AssertEq(t, string(content), expected)
		return
	}
}
