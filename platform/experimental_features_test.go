package platform_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/platform"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	llog "github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExperimentalFeatures(t *testing.T) {
	spec.Run(t, "ExperimentalFeatures", testExperimentalFeatures, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testExperimentalFeatures(t *testing.T, when spec.G, it spec.S) {
	var (
		logHandler *memory.Handler
		logger     llog.Logger
	)

	it.Before(func() {
		logHandler = memory.New()
		logger = &log.Logger{Handler: logHandler}
	})

	when("GuardExperimental", func() {
		when("CNB_PLATFORM_EXPERIMENTAL_FEATURES=warn", func() {
			it("warns", func() {
				platform.ExperimentalMode = platform.ExperimentalModeWarn
				err := platform.GuardExperimental("some-feature", logger)
				h.AssertNil(t, err)
				h.AssertEq(t, len(logHandler.Entries), 1)
				h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
				h.AssertEq(t, logHandler.Entries[0].Message, "Platform requested experimental feature 'some-feature'")
			})
		})

		when("CNB_PLATFORM_EXPERIMENTAL_FEATURES=quiet", func() {
			it("succeeds silently", func() {
				platform.ExperimentalMode = platform.ExperimentalModeQuiet
				err := platform.GuardExperimental("some-feature", logger)
				h.AssertNil(t, err)
				h.AssertEq(t, len(logHandler.Entries), 0)
			})
		})

		when("CNB_PLATFORM_EXPERIMENTAL_FEATURES=error", func() {
			it("error with exit code 11", func() {
				platform.ExperimentalMode = platform.ExperimentalModeError
				err := platform.GuardExperimental("some-feature", logger)
				h.AssertNotNil(t, err)
				h.AssertEq(t, len(logHandler.Entries), 2)
				h.AssertEq(t, logHandler.Entries[0].Level, log.ErrorLevel)
				h.AssertEq(t, logHandler.Entries[0].Message, "Platform requested experimental feature 'some-feature'")
				h.AssertEq(t, logHandler.Entries[1].Level, log.ErrorLevel)
				h.AssertEq(t, logHandler.Entries[1].Message, "Experimental features are disabled by CNB_PLATFORM_EXPERIMENTAL_FEATURES=error")
			})
		})
	})
}
