package env_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	penv "github.com/buildpacks/lifecycle/platform/env"
	lenv "github.com/buildpacks/lifecycle/platform/launch/env"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestEnv(t *testing.T) {
	spec.Run(t, "Env", testEnv, spec.Report(report.Terminal{}))
}

func testEnv(t *testing.T, when spec.G, it spec.S) {
	it("values match the platform package", func() {
		h.AssertEq(t, lenv.VarAppDir, penv.VarAppDir)
		h.AssertEq(t, lenv.VarDeprecationMode, penv.VarDeprecationMode)
		h.AssertEq(t, lenv.VarLayersDir, penv.VarLayersDir)
		h.AssertEq(t, lenv.VarNoColor, penv.VarNoColor)
		h.AssertEq(t, lenv.VarPlatformAPI, penv.VarPlatformAPI)
		h.AssertEq(t, lenv.VarProcessType, penv.VarProcessType)
	})
}
