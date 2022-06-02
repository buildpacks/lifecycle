package guard_test

import (
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/guard"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExperimentalFeature(t *testing.T) {
	spec.Run(t, "Feature", testExperimentalFeature, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testExperimentalFeature(t *testing.T, when spec.G, it spec.S) {
	var (
		logger     llog.Logger
		logHandler *memory.Handler
	)

	it.Before(func() {
		logHandler = memory.New()
		logger = &log.Logger{Handler: logHandler}
	})

	when("ExperimentalFeature", func() {
		when("CNB_PLATFORM_EXPERIMENTAL_MODE=warn", func() {
			it("warns", func() {
				guard.ExperimentalFeaturesMode = "warn"
				h.AssertNil(t, guard.ExperimentalFeature("some-feature", logger))
				h.AssertEq(t, len(logHandler.Entries), 1)
				h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
				h.AssertEq(t, logHandler.Entries[0].Message, "Experimental feature 'some-feature' requested")
			})
		})

		when("CNB_PLATFORM_EXPERIMENTAL_MODE=quiet", func() {
			it("succeeds silently", func() {
				guard.ExperimentalFeaturesMode = "quiet"
				h.AssertNil(t, guard.ExperimentalFeature("some-feature", logger))
				h.AssertEq(t, len(logHandler.Entries), 0)
			})
		})

		when("CNB_PLATFORM_EXPERIMENTAL_MODE=error", func() {
			it("errors", func() {
				guard.ExperimentalFeaturesMode = "error"
				err := guard.ExperimentalFeature("some-feature", logger)
				h.AssertEq(t, len(logHandler.Entries), 1)
				h.AssertEq(t, logHandler.Entries[0].Level, log.ErrorLevel)
				h.AssertEq(t, logHandler.Entries[0].Message, "Experimental feature 'some-feature' requested")
				h.AssertEq(t, err.Error(), "Experimental features are disabled by CNB_PLATFORM_EXPERIMENTAL_MODE=error")
			})
		})
	})
}
