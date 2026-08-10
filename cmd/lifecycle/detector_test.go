package main

import (
	"errors"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestUnwrapErrorFailWithCode(t *testing.T) {
	spec.Run(t, "unwrapErrorFailWithCode", testUnwrapErrorFailWithCode, spec.Report(report.Terminal{}))
}

func testUnwrapErrorFailWithCode(t *testing.T, when spec.G, it spec.S) {
	when("err is a plain error", func() {
		it("wraps it with the provided fallback exit code, instead of always using the generic failure code", func() {
			// simulates the error returned when registry read access validation fails during analyzer initialization,
			// as reported in https://github.com/buildpacks/lifecycle/issues/1572
			registryErr := errors.New("validating registry read access: failed to ensure registry read access to some-image: DENIED")
			analyzeErrorCode := platform.NewExiter("0.99").CodeFor(platform.AnalyzeError)

			// prior to the fix, initialize-analyzer failures always went through unwrapErrorFailWithMessage,
			// which hard-codes cmd.CodeForFailed regardless of which phase produced the error
			generic := unwrapErrorFailWithMessage(registryErr, "initialize analyzer")
			h.AssertEq(t, generic.(*cmd.ErrorFail).Code, cmd.CodeForFailed)

			// the fix routes the same error through unwrapErrorFailWithCode with the analyze phase's exit code (30-39 range),
			// instead of the generic exit code 1
			fixed := unwrapErrorFailWithCode(registryErr, analyzeErrorCode, "initialize analyzer")
			failErr, ok := fixed.(*cmd.ErrorFail)
			if !ok {
				t.Fatalf("expected an error of type *cmd.ErrorFail")
			}
			h.AssertEq(t, failErr.Code, analyzeErrorCode)
			h.AssertError(t, fixed, "failed to initialize analyzer: "+registryErr.Error())
		})
	})

	when("err is already a *cmd.ErrorFail", func() {
		it("preserves the original exit code instead of the fallback", func() {
			original := cmd.FailErrCode(errors.New("some failure"), cmd.CodeForInvalidArgs, "resolve inputs")

			err := unwrapErrorFailWithCode(original, platform.NewExiter("0.99").CodeFor(platform.AnalyzeError), "initialize analyzer")

			failErr, ok := err.(*cmd.ErrorFail)
			if !ok {
				t.Fatalf("expected an error of type *cmd.ErrorFail")
			}
			h.AssertEq(t, failErr.Code, cmd.CodeForInvalidArgs)
		})
	})
}
