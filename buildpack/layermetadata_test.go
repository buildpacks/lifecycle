package buildpack_test

import (
	"os"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLayerMetadata(t *testing.T) {
	spec.Run(t, "unit-layermetadata", testLayerMetadata, spec.Report(report.Terminal{}))
}

func testLayerMetadata(t *testing.T, when spec.G, it spec.S) {
	when("#LayerMetadata", func() {
		var (
			metadataFile *os.File
		)
		it.Before(func() {
			var err error
			metadataFile, err = os.CreateTemp("", "test")
			h.AssertNil(t, err)
		})
		it.After(func() {
			os.Remove(metadataFile.Name())
		})
		it("decodes file correctly", func() {
			err := os.WriteFile(metadataFile.Name(), []byte("[types]\ncache = true"), 0400)
			h.AssertNil(t, err)

			var lmf buildpack.LayerMetadataFile
			lmf, s, err := buildpack.DecodeLayerMetadataFile(metadataFile.Name(), "0.9")
			h.AssertNil(t, err)
			h.AssertEq(t, s, "")
			h.AssertEq(t, lmf.Cache, true)
			h.AssertEq(t, lmf.Build, false)
			h.AssertEq(t, lmf.Launch, false)
		})
	})
}
