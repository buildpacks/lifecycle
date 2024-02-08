package layers_test

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpacks/imgutil"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestTarLayer(t *testing.T) {
	spec.Run(t, "TarLayer", testTarLayer, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testTarLayer(t *testing.T, when spec.G, it spec.S) {
	when("provided layer tar", func() {
		var factory *layers.Factory

		it.Before(func() {
			artifactDir, err := os.MkdirTemp("", "lifecycle.layers.tar.artifacts")
			h.AssertNil(t, err)
			factory = &layers.Factory{ArtifactsDir: artifactDir}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(factory.ArtifactsDir))
		})

		when("compressed", func() {
			it("zeros timestamps on the provided layer", func() {
				fromPath := filepath.Join("testdata", "some-compressed-layer.tgz")

				layer, err := factory.TarLayer("some-extension-id:some-layer-name", fromPath, "some-created-by")
				h.AssertNil(t, err)

				assertTimestamps(t, layer.TarPath, imgutil.NormalizedDateTime)
				h.AssertEq(t, layer.ID, "some-extension-id:some-layer-name")
				h.AssertEq(t, layer.TarPath, filepath.Join(factory.ArtifactsDir, "some-extension-id:some-layer-name.tar"))
				h.AssertEq(t, layer.Digest, "sha256:b3fe551e475c67de98decc610589739691fbb829fdd437107a964924ca3acae8") // from fixture
				h.AssertEq(t, layer.History, v1.History{CreatedBy: "some-created-by"})
			})
		})

		when("uncompressed", func() {
			it("zeros timestamps on the provided layer", func() {
				fromPath := filepath.Join("testdata", "some-uncompressed-layer.tar")

				layer, err := factory.TarLayer("some-extension-id:some-layer-name", fromPath, "some-created-by")
				h.AssertNil(t, err)

				assertTimestamps(t, layer.TarPath, imgutil.NormalizedDateTime)
				h.AssertEq(t, layer.ID, "some-extension-id:some-layer-name")
				h.AssertEq(t, layer.TarPath, filepath.Join(factory.ArtifactsDir, "some-extension-id:some-layer-name.tar"))
				h.AssertEq(t, layer.Digest, "sha256:b3fe551e475c67de98decc610589739691fbb829fdd437107a964924ca3acae8") // from fixture
				h.AssertEq(t, layer.History, v1.History{CreatedBy: "some-created-by"})
			})
		})
	})
}

func assertTimestamps(t *testing.T, tarPath string, expected time.Time) {
	t.Helper()

	f, err := os.Open(tarPath) // #nosec G304
	h.AssertNil(t, err)
	defer f.Close() // nolint

	tr := tar.NewReader(f)
	var found int
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)
		found++
		h.AssertEq(t, header.ModTime, expected)
	}
	h.AssertEq(t, found > 0, true)
}
