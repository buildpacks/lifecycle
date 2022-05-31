package modes_test

import (
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/modes"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAPIVerifier(t *testing.T) {
	spec.Run(t, "APIVerifier", testAPIVerifier, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testAPIVerifier(t *testing.T, when spec.G, it spec.S) {
	var (
		logger     *cmd.Logger
		logHandler *memory.Handler
		supported  api.APIs
	)

	it.Before(func() {
		logHandler = memory.New()
		logger = &cmd.Logger{Logger: &log.Logger{Handler: logHandler}}
	})

	when("VerifyPlatformAPI", func() {
		it.Before(func() {
			var err error
			supported, err = api.NewAPIs([]string{"1.2", "2.1"}, []string{"1"})
			h.AssertNil(t, err)
		})

		when("is invalid", func() {
			it("errors", func() {
				err := modes.VerifyPlatformAPI("bad-api", supported, logger)
				h.AssertNotNil(t, err)
			})
		})

		when("is unsupported", func() {
			it("errors", func() {
				err := modes.VerifyPlatformAPI("2.2", supported, logger)
				h.AssertNotNil(t, err)
			})
		})

		when("is deprecated", func() {
			when("CNB_DEPRECATION_MODE=warn", func() {
				it("warns", func() {
					modes.Deprecation = modes.Warn
					err := modes.VerifyPlatformAPI("1.1", supported, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Platform requested deprecated API '1.1'")
				})
			})

			when("CNB_DEPRECATION_MODE=quiet", func() {
				it("succeeds silently", func() {
					modes.Deprecation = modes.Quiet
					err := modes.VerifyPlatformAPI("1.1", supported, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_DEPRECATION_MODE=error", func() {
				it("errors", func() {
					modes.Deprecation = modes.Error
					err := modes.VerifyPlatformAPI("1.1", supported, logger)
					h.AssertNotNil(t, err)
				})
			})
		})

		when("is experimental", func() {
			when("CNB_EXPERIMENTAL_MODE=warn", func() {
				it("warns", func() {
					modes.ExperimentalAPIs = modes.Warn
					err := modes.VerifyPlatformAPI("2.1-alpha-1", supported, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Platform requested experimental API '2.1-alpha-1'")
				})
			})

			when("CNB_EXPERIMENTAL_MODE=quiet", func() {
				it("succeeds silently", func() {
					modes.ExperimentalAPIs = modes.Quiet
					err := modes.VerifyPlatformAPI("2.1-alpha-1", supported, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_EXPERIMENTAL_MODE=error", func() {
				it("errors", func() {
					modes.ExperimentalAPIs = modes.Error
					err := modes.VerifyPlatformAPI("2.1-alpha-1", supported, logger)
					h.AssertNotNil(t, err)
				})
			})
		})
	})

	when("VerifyBuildpackAPIs", func() {
		it.Before(func() {
			var err error
			supported, err = api.NewAPIs([]string{"1.2", "2.1"}, []string{"1"})
			h.AssertNil(t, err)
		})

		when("is invalid", func() {
			it("errors", func() {
				err := modes.VerifyBuildpackAPI("some-buildpack", "bad-api", supported, logger)
				h.AssertNotNil(t, err)
			})
		})

		when("is unsupported", func() {
			it("errors", func() {
				err := modes.VerifyBuildpackAPI("some-buildpack", "2.2", supported, logger)
				h.AssertNotNil(t, err)
			})
		})

		when("is deprecated", func() {
			when("CNB_DEPRECATION_MODE=warn", func() {
				it("warns", func() {
					modes.Deprecation = modes.Warn
					err := modes.VerifyBuildpackAPI("some-buildpack", "1.1", supported, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Buildpack 'some-buildpack' requests deprecated API '1.1'")
				})
			})

			when("CNB_DEPRECATION_MODE=quiet", func() {
				it("succeeds silently", func() {
					modes.Deprecation = modes.Quiet
					err := modes.VerifyBuildpackAPI("some-buildpack", "1.1", supported, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_DEPRECATION_MODE=error", func() {
				it("errors", func() {
					modes.Deprecation = modes.Error
					err := modes.VerifyBuildpackAPI("some-buildpack", "1.1", supported, logger)
					h.AssertNotNil(t, err)
				})
			})
		})

		when("is experimental", func() {
			when("CNB_EXPERIMENTAL_MODE=warn", func() {
				it("warns", func() {
					modes.ExperimentalAPIs = modes.Warn
					err := modes.VerifyBuildpackAPI("some-buildpack", "2.1-alpha-1", supported, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Buildpack 'some-buildpack' requests experimental API '2.1-alpha-1'")
				})
			})

			when("CNB_EXPERIMENTAL_MODE=quiet", func() {
				it("succeeds silently", func() {
					modes.ExperimentalAPIs = modes.Quiet
					err := modes.VerifyBuildpackAPI("some-buildpack", "2.1-alpha-1", supported, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_EXPERIMENTAL_MODE=error", func() {
				it("errors", func() {
					modes.ExperimentalAPIs = modes.Error
					err := modes.VerifyBuildpackAPI("some-buildpack", "2.1-alpha-1", supported, logger)
					h.AssertNotNil(t, err)
				})
			})
		})
	})
}
