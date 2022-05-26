package cmd_test

import (
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAPIVerifier(t *testing.T) {
	spec.Run(t, "APIVerifier", testAPIVerifier, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testAPIVerifier(t *testing.T, when spec.G, it spec.S) {
	var (
		logHandler *memory.Handler
	)

	it.Before(func() {
		logHandler = memory.New()
		cmd.DefaultLogger = &cmd.Logger{Logger: &log.Logger{Handler: logHandler}}
	})

	when("VerifyPlatformAPI", func() {
		it.Before(func() {
			var err error
			api.Platform, err = api.NewAPIs([]string{"1.2", "2.1"}, []string{"1"})
			h.AssertNil(t, err)
		})

		when("is invalid", func() {
			it("errors with exit code 11", func() {
				err := cmd.VerifyPlatformAPI("bad-api")
				failErr, ok := err.(*cmd.ErrorFail)
				if !ok {
					t.Fatalf("expected an error of type cmd.ErrorFail")
				}
				h.AssertEq(t, failErr.Code, 11)
			})
		})

		when("is unsupported", func() {
			it("errors with exit code 11", func() {
				err := cmd.VerifyPlatformAPI("2.2")
				failErr, ok := err.(*cmd.ErrorFail)
				if !ok {
					t.Fatalf("expected an error of type cmd.ErrorFail")
				}
				h.AssertEq(t, failErr.Code, 11)
			})
		})

		when("is deprecated", func() {
			when("CNB_DEPRECATION_MODE=warn", func() {
				it("warns", func() {
					cmd.DeprecationMode = cmd.ModeWarn
					err := cmd.VerifyPlatformAPI("1.1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Platform requested deprecated API '1.1'")
				})
			})

			when("CNB_DEPRECATION_MODE=quiet", func() {
				it("succeeds silently", func() {
					cmd.DeprecationMode = cmd.ModeQuiet
					err := cmd.VerifyPlatformAPI("1.1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_DEPRECATION_MODE=error", func() {
				it("errors with exit code 11", func() {
					cmd.DeprecationMode = cmd.ModeError
					err := cmd.VerifyPlatformAPI("1.1")
					failErr, ok := err.(*cmd.ErrorFail)
					if !ok {
						t.Fatalf("expected an error of type cmd.ErrorFail")
					}
					h.AssertEq(t, failErr.Code, 11)
				})
			})
		})

		when("is experimental", func() {
			when("CNB_EXPERIMENTAL_MODE=warn", func() {
				it("warns", func() {
					cmd.ExperimentalMode = cmd.ModeWarn
					err := cmd.VerifyPlatformAPI("2.1-alpha-1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Platform requested experimental API '2.1-alpha-1'")
				})
			})

			when("CNB_EXPERIMENTAL_MODE=quiet", func() {
				it("succeeds silently", func() {
					cmd.ExperimentalMode = cmd.ModeQuiet
					err := cmd.VerifyPlatformAPI("2.1-alpha-1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_EXPERIMENTAL_MODE=error", func() {
				it("errors with exit code 11", func() {
					cmd.ExperimentalMode = cmd.ModeError
					err := cmd.VerifyPlatformAPI("2.1-alpha-1")
					failErr, ok := err.(*cmd.ErrorFail)
					if !ok {
						t.Fatalf("expected an error of type cmd.ErrorFail")
					}
					h.AssertEq(t, failErr.Code, 11)
				})
			})
		})
	})

	when("VerifyBuildpackAPIs", func() {
		it.Before(func() {
			var err error
			api.Buildpack, err = api.NewAPIs([]string{"1.2", "2.1"}, []string{"1"})
			h.AssertNil(t, err)
		})

		when("is invalid", func() {
			it("error with exit code 12", func() {
				err := cmd.VerifyBuildpackAPI("some-buildpack", "bad-api")
				failErr, ok := err.(*cmd.ErrorFail)
				if !ok {
					t.Fatalf("expected an error of type cmd.ErrorFail")
				}
				h.AssertEq(t, failErr.Code, 12)
			})
		})

		when("is unsupported", func() {
			it("errors with exit code 11", func() {
				err := cmd.VerifyBuildpackAPI("some-buildpack", "2.2")
				failErr, ok := err.(*cmd.ErrorFail)
				if !ok {
					t.Fatalf("expected an error of type cmd.ErrorFail")
				}
				h.AssertEq(t, failErr.Code, 12)
			})
		})

		when("is deprecated", func() {
			when("CNB_DEPRECATION_MODE=warn", func() {
				it("warns", func() {
					cmd.DeprecationMode = cmd.ModeWarn
					err := cmd.VerifyBuildpackAPI("some-buildpack", "1.1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Buildpack 'some-buildpack' requests deprecated API '1.1'")
				})
			})

			when("CNB_DEPRECATION_MODE=quiet", func() {
				it("succeeds silently", func() {
					cmd.DeprecationMode = cmd.ModeQuiet
					err := cmd.VerifyBuildpackAPI("some-buildpack", "1.1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_DEPRECATION_MODE=error", func() {
				it("errors with exit code 11", func() {
					cmd.DeprecationMode = cmd.ModeError
					err := cmd.VerifyBuildpackAPI("some-buildpack", "1.1")
					failErr, ok := err.(*cmd.ErrorFail)
					if !ok {
						t.Fatalf("expected an error of type cmd.ErrorFail")
					}
					h.AssertEq(t, failErr.Code, 12)
				})
			})
		})

		when("is experimental", func() {
			when("CNB_EXPERIMENTAL_MODE=warn", func() {
				it("warns", func() {
					cmd.ExperimentalMode = cmd.ModeWarn
					err := cmd.VerifyBuildpackAPI("some-buildpack", "2.1-alpha-1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Buildpack 'some-buildpack' requests experimental API '2.1-alpha-1'")
				})
			})

			when("CNB_EXPERIMENTAL_MODE=quiet", func() {
				it("succeeds silently", func() {
					cmd.ExperimentalMode = cmd.ModeQuiet
					err := cmd.VerifyBuildpackAPI("some-buildpack", "2.1-alpha-1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_EXPERIMENTAL_MODE=error", func() {
				it("errors with exit code 11", func() {
					cmd.ExperimentalMode = cmd.ModeError
					err := cmd.VerifyBuildpackAPI("some-buildpack", "2.1-alpha-1")
					failErr, ok := err.(*cmd.ErrorFail)
					if !ok {
						t.Fatalf("expected an error of type cmd.ErrorFail")
					}
					h.AssertEq(t, failErr.Code, 12)
				})
			})
		})
	})
}
