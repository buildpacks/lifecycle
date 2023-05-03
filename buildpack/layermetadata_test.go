package buildpack_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/apex/log"

	"github.com/apex/log/handlers/memory"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	llog "github.com/buildpacks/lifecycle/log"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var logHandler *memory.Handler
var logger llog.Logger

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
			logHandler = memory.New()
			logger = &log.Logger{Handler: logHandler}
		})
		it.After(func() {
			os.Remove(metadataFile.Name())
		})
		it("decodes file correctly", func() {
			err := os.WriteFile(metadataFile.Name(), []byte("[types]\ncache = true"), 0400)
			h.AssertNil(t, err)

			var lmf buildpack.LayerMetadataFile
			lmf, err = buildpack.DecodeLayerMetadataFile(metadataFile.Name(), "0.9", logger)
			h.AssertNil(t, err)
			h.AssertEq(t, lmf.Cache, true)
			h.AssertEq(t, lmf.Build, false)
			h.AssertEq(t, lmf.Launch, false)
		})
		it("logs a warning when the metadata file has wrong format (on older apis)", func() {
			err := os.WriteFile(metadataFile.Name(), []byte("cache = true"), 0400)
			h.AssertNil(t, err)
			var lmf buildpack.LayerMetadataFile
			lmf, err = buildpack.DecodeLayerMetadataFile(metadataFile.Name(), "0.5", logger)
			h.AssertNil(t, err)
			h.AssertEq(t, lmf.Cache, false)
			h.AssertEq(t, lmf.Build, false)
			h.AssertEq(t, lmf.Launch, false)
			expected := fmt.Sprintf("the launch, cache and build flags should be in the types table of %s", metadataFile.Name())
			h.AssertLogEntry(t, logHandler, expected)
		})
		it("returns an error when the metadata file has wrong format", func() {
			err := os.WriteFile(metadataFile.Name(), []byte("cache = true"), 0400)
			h.AssertNil(t, err)

			var lmf buildpack.LayerMetadataFile
			lmf, err = buildpack.DecodeLayerMetadataFile(metadataFile.Name(), "0.9", logger)
			h.AssertNotNil(t, err)
			h.AssertEq(t, lmf.Cache, false)
			h.AssertEq(t, lmf.Build, false)
			h.AssertEq(t, lmf.Launch, false)
		})
	})
}
