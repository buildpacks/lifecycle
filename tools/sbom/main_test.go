package main

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSbomGenerator(t *testing.T) {
	spec.Run(t, "SbomGenerator", testSbomGenerator, spec.Report(report.Terminal{}))
}

func testSbomGenerator(t *testing.T, when spec.G, it spec.S) {
	when("when given a directory with a simple GO binary", func() {
		var (
			tmpDir                  string
			simpleAppBinaryFullpath string
		)
		simpleAppBinaryFilename := "simple-app"

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "simple-go-app")
			h.AssertNil(t, err)

			src := filepath.Join("..", "..", "acceptance", "testdata", "simple-go-app", simpleAppBinaryFilename)
			h.AssertPathExists(t, src)

			simpleAppBinaryFullpath = filepath.Join(tmpDir, simpleAppBinaryFilename)

			fin, err := os.Open(src)
			if err != nil {
				t.Fatalf("failed to copy file to hasher: %s", err)
			}
			defer fin.Close()

			fout, err := os.Create(simpleAppBinaryFullpath)
			if err != nil {
				t.Fatalf("failed to copy file to hasher: %s", err)
			}
			defer fout.Close()

			_, err = io.Copy(fout, fin)
			h.AssertNil(t, err)
			h.AssertPathExists(t, simpleAppBinaryFullpath)
		})

		it("generates an SBOM for that binary", func() {
			errors := GenerateSBOM(simpleAppBinaryFullpath)

			h.AssertEq(t, len(errors), 3)

			for _, err := range errors {
				h.AssertNil(t, err)
			}

			h.AssertPathExists(t, filepath.Join(tmpDir, "simple-app.sbom.cdx.json"))
			h.AssertPathExists(t, filepath.Join(tmpDir, "simple-app.sbom.spdx.json"))
			h.AssertPathExists(t, filepath.Join(tmpDir, "simple-app.sbom.syft.json"))

			cdxContent, _ := ioutil.ReadFile(filepath.Join(tmpDir, "simple-app.sbom.cdx.json"))
			spdxContent, _ := ioutil.ReadFile(filepath.Join(tmpDir, "simple-app.sbom.spdx.json"))
			syftContent, _ := ioutil.ReadFile(filepath.Join(tmpDir, "simple-app.sbom.syft.json"))

			h.AssertStringContains(t, string(cdxContent), "github.com/pkg/errors")
			h.AssertStringContains(t, string(cdxContent), "CycloneDX")

			h.AssertStringContains(t, string(spdxContent), "github.com/pkg/errors")
			h.AssertStringContains(t, string(spdxContent), "spdxVersion")

			h.AssertStringContains(t, string(syftContent), "github.com/pkg/errors")
			h.AssertStringContains(t, string(syftContent), "https://raw.githubusercontent.com/anchore/syft/")
		})
	})

	when("when given a directory as parameter", func() {
		var (
			tmpDir string
		)

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "simple-go-app")
			h.AssertNil(t, err)
		})

		it("returns an array with one error", func() {
			errors := GenerateSBOM(tmpDir)

			h.AssertEq(t, len(errors), 1)
			h.AssertNotNil(t, errors[0])
		})
	})

	when("when given an absolute path to a binary file with an executable extension .exe", func() {
		it("returns the filename without the extension", func() {
			h.AssertEq(t, GenerateFilename("/windows-amd64/lifecycle/lifecycle.exe"), "lifecycle")
		})
	})

	when("when given an absolute path to a binary file without an extension", func() {
		it("returns the filename without the extension", func() {
			h.AssertEq(t, GenerateFilename("/usr/local/bin/test"), "test")
		})
	})
}
