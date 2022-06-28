package cmd_test

import (
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPlatformAPI(t *testing.T) {
	spec.Run(t, "PlatformAPI", testPlatformAPI, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testPlatformAPI(t *testing.T, when spec.G, it spec.S) {
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
			it("error with exit code 11", func() {
				err := cmd.VerifyPlatformAPI("bad-api")
				failErr, ok := err.(*cmd.ErrorFail)
				if !ok {
					t.Fatalf("expected an error of type cmd.ErrorFail")
				}
				h.AssertEq(t, failErr.Code, 11)
			})
		})

		when("is unsupported", func() {
			it("error with exit code 11", func() {
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
				it("should warn", func() {
					cmd.DeprecationMode = cmd.DeprecationModeWarn
					err := cmd.VerifyPlatformAPI("1.1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Platform requested deprecated API '1.1'")
				})
			})

			when("CNB_DEPRECATION_MODE=quiet", func() {
				it("should succeed silently", func() {
					cmd.DeprecationMode = cmd.DeprecationModeQuiet
					err := cmd.VerifyPlatformAPI("1.1")
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_DEPRECATION_MODE=error", func() {
				it("error with exit code 11", func() {
					cmd.DeprecationMode = cmd.DeprecationModeError
					err := cmd.VerifyPlatformAPI("1.1")
					failErr, ok := err.(*cmd.ErrorFail)
					if !ok {
						t.Fatalf("expected an error of type cmd.ErrorFail")
					}
					h.AssertEq(t, failErr.Code, 11)
				})
			})
		})
	})

	when("VerifyBuildpackAPI", func() {
		it.Before(func() {
			var err error
			api.Buildpack, err = api.NewAPIs([]string{"1.2", "2.1"}, []string{"1"})
			h.AssertNil(t, err)
		})

		when("is invalid", func() {
			it("error with exit code 12", func() {
				err := cmd.VerifyBuildpackAPI(buildpack.KindBuildpack, "some-buildpack", "bad-api", cmd.DefaultLogger)
				failErr, ok := err.(*cmd.ErrorFail)
				if !ok {
					t.Fatalf("expected an error of type cmd.ErrorFail")
				}
				h.AssertEq(t, failErr.Code, 12)
			})
		})

		when("is unsupported", func() {
			it("error with exit code 11", func() {
				err := cmd.VerifyBuildpackAPI(buildpack.KindBuildpack, "some-buildpack", "2.2", cmd.DefaultLogger)
				failErr, ok := err.(*cmd.ErrorFail)
				if !ok {
					t.Fatalf("expected an error of type cmd.ErrorFail")
				}
				h.AssertEq(t, failErr.Code, 12)
			})
		})

		when("is deprecated", func() {
			when("CNB_DEPRECATION_MODE=warn", func() {
				it("should warn", func() {
					cmd.DeprecationMode = cmd.DeprecationModeWarn
					err := cmd.VerifyBuildpackAPI(buildpack.KindBuildpack, "some-buildpack", "1.1", cmd.DefaultLogger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 1)
					h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
					h.AssertEq(t, logHandler.Entries[0].Message, "Buildpack 'some-buildpack' requests deprecated API '1.1'")
				})
			})

			when("CNB_DEPRECATION_MODE=quiet", func() {
				it("should succeed silently", func() {
					cmd.DeprecationMode = cmd.DeprecationModeQuiet
					err := cmd.VerifyBuildpackAPI(buildpack.KindBuildpack, "some-buildpack", "1.1", cmd.DefaultLogger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(logHandler.Entries), 0)
				})
			})

			when("CNB_DEPRECATION_MODE=error", func() {
				it("error with exit code 11", func() {
					cmd.DeprecationMode = cmd.DeprecationModeError
					err := cmd.VerifyBuildpackAPI(buildpack.KindBuildpack, "some-buildpack", "1.1", cmd.DefaultLogger)
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
