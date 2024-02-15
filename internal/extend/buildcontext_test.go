package extend_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestContext(t *testing.T) {
	spec.Run(t, "unit-context", testContext, spec.Report(report.Terminal{}))
}

func testContext(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string
	var logger log.Logger

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "contexts")
		logger = log.NewDefaultLogger(io.Discard)

		h.AssertNil(t, err)
	})

	it.After(func() {
		_ = os.RemoveAll(tmpDir)
	})

	when("#FindContexts", func() {
		when("only shared context is provided", func() {
			it.Before(func() {
				h.Mkdir(t, filepath.Join(tmpDir, extend.SharedContextDir))
			})

			it("succeeds", func() {
				contexts, err := extend.FindContexts("A", tmpDir, logger)
				h.AssertNil(t, err)

				h.AssertEq(t, len(contexts), 1)
				h.AssertEq(t, contexts[0], extend.ContextInfo{
					ExtensionID: "A",
					Path:        filepath.Join(tmpDir, extend.SharedContextDir),
				})
			})
		})

		when("image specific contexts are provided", func() {
			it.Before(func() {
				h.Mkdir(t, filepath.Join(tmpDir, extend.RunContextDir), filepath.Join(tmpDir, extend.BuildContextDir))
			})

			it("succeeds", func() {
				contexts, err := extend.FindContexts("A", tmpDir, logger)
				h.AssertNil(t, err)

				h.AssertEq(t, len(contexts), 2)
				h.AssertEq(t, contexts[0], extend.ContextInfo{
					ExtensionID: "A",
					Path:        filepath.Join(tmpDir, extend.BuildContextDir),
				})
				h.AssertEq(t, contexts[1], extend.ContextInfo{
					ExtensionID: "A",
					Path:        filepath.Join(tmpDir, extend.RunContextDir),
				})
			})
		})

		when("no context is provided", func() {
			it("succeeds", func() {
				contexts, err := extend.FindContexts("A", tmpDir, logger)
				h.AssertNil(t, err)

				h.AssertEq(t, len(contexts), 0)
			})
		})

		when("shared and image-specific contexts are provided", func() {
			it.Before(func() {
				h.Mkdir(t, filepath.Join(tmpDir, extend.SharedContextDir), filepath.Join(tmpDir, extend.BuildContextDir))
			})

			it("fails", func() {
				_, err := extend.FindContexts("A", tmpDir, logger)
				h.AssertNotNil(t, err)
			})
		})
	})
}
