package extend_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/internal/extend"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestGenerate(t *testing.T) {
	spec.Run(t, "unit-extend", testConfig, spec.Report(report.Terminal{}))
}

func testConfig(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle")
		h.AssertNil(t, err)
	})

	it.After(func() {
		_ = os.RemoveAll(tmpDir)
	})

	when("#ValidateConfig", func() {
		when("valid", func() {
			it("succeeds", func() {
				data := `
[[build.args]]
name = "some-build-base-arg-name"
value = "some-build-base-arg-value"

[[run.args]]
name = "some-run-base-arg-name"
value = "some-run-base-arg-value"
`
				config := filepath.Join(tmpDir, "extend-config.toml")
				h.Mkfile(t, data, config)
				h.AssertNil(t, extend.ValidateConfig(config))
			})
		})

		when("invalid", func() {
			when("contains disallowed build arg", func() {
				it("errors", func() {
					invalidContent := []string{
						`
						[[build.args]]
						name = "build_id" # invalid
						value = "some-build-base-arg-value"
						[[run.args]]
						name = "some-run-base-arg-name"
						value = "some-run-base-arg-value"
						`,
						`
						[[build.args]]
						name = "some-build-base-arg-name"
						value = "some-build-base-arg-value"
						[[run.args]]
						name = "user_id" # invalid
						value = "some-run-base-arg-value"
						`,
					}
					for _, c := range invalidContent {
						config := filepath.Join(tmpDir, "extend-config.toml")
						h.Mkfile(t, c, config)
						h.AssertNotNil(t, extend.ValidateConfig(config))
					}
				})
			})
		})
	})
}
