package launch_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestProcess(t *testing.T) {
	spec.Run(t, "Process", testProcess, spec.Report(report.Terminal{}))
}

// RawCommandValue should be ignored because it is a toml.Primitive that has not been exported.
var processCmpOpts = []cmp.Option{
	cmpopts.IgnoreFields(launch.Process{}, "RawCommandValue", "PlatformAPI"),
}

func testProcess(t *testing.T, when spec.G, it spec.S) {
	var (
		launcher *launch.Launcher
	)
	it.Before(func() {
		launcher = &launch.Launcher{
			Processes: []launch.Process{
				{
					Type:        "some-type",
					Command:     []string{"some-command"},
					Args:        []string{"some-arg1", "some-arg2"},
					BuildpackID: "some-buildpack",
				},
				{
					Type:        "other-type",
					Command:     []string{"other-command"},
					Args:        []string{"other-arg1", "other-arg2"},
					BuildpackID: "some-buildpack",
				},
				{
					Type:        "type-with-default-args",
					Command:     []string{"some-command", "some-arg1"},
					Args:        []string{"some-arg2"},
					BuildpackID: "some-buildpack",
				},
			},
			PlatformAPI: api.Platform.Latest(),
		}
	})

	when("ProcessFor", func() {
		when("DefaultProcessType", func() {
			when("is unset", func() {
				when("cmd starts with --", func() {
					it("creates a new direct process from cmd", func() {
						proc, err := launcher.ProcessFor([]string{"--", "user-command", "user-arg1", "user-arg2"})
						h.AssertNil(t, err)
						h.AssertEq(t, proc, launch.Process{
							Command: []string{"user-command"},
							Args:    []string{"user-arg1", "user-arg2"},
							Direct:  true,
						}, processCmpOpts...)
					})
				})

				when("cmd does not start with --", func() {
					it("creates a new shell process from cmd", func() {
						proc, err := launcher.ProcessFor([]string{"user-command", "user-arg1", "user-arg2"})
						h.AssertNil(t, err)
						h.AssertEq(t, proc, launch.Process{
							Command: []string{"user-command"},
							Args:    []string{"user-arg1", "user-arg2"},
						}, processCmpOpts...)
					})
				})

				when("cmd is empty", func() {
					it("errors", func() {
						_, err := launcher.ProcessFor([]string{})
						h.AssertNotNil(t, err)
					})
				})
			})

			when("exists", func() {
				when("process has default arguments", func() {
					it.Before(func() {
						launcher.DefaultProcessType = "type-with-default-args"
					})

					it("provides command args and replaces default args with cmd", func() {
						proc, err := launcher.ProcessFor([]string{"user-arg1", "user-arg2"})
						h.AssertNil(t, err)
						h.AssertEq(t, proc, launch.Process{
							Type:        "type-with-default-args",
							Command:     []string{"some-command"},
							Args:        []string{"some-arg1", "user-arg1", "user-arg2"},
							BuildpackID: "some-buildpack",
						}, processCmpOpts...)
					})
				})

				when("process does not have default arguments", func() {
					it.Before(func() {
						launcher.DefaultProcessType = "some-type"
					})

					it("appends cmd to the default process args", func() {
						proc, err := launcher.ProcessFor([]string{"user-arg1", "user-arg1"})
						h.AssertNil(t, err)
						h.AssertEq(t, proc, launch.Process{
							Type:        "some-type",
							Command:     []string{"some-command"},
							Args:        []string{"some-arg1", "some-arg2", "user-arg1", "user-arg1"},
							BuildpackID: "some-buildpack",
						}, processCmpOpts...)
					})
				})
			})

			when("doesn't exist", func() {
				it.Before(func() {
					launcher.DefaultProcessType = "missing-type"
				})

				it("errors", func() {
					_, err := launcher.ProcessFor([]string{"user-arg1", "user-arg1"})
					h.AssertNotNil(t, err)
				})
			})
		})

		when("Platform API < 0.10", func() {
			it.Before(func() {
				launcher.PlatformAPI = api.MustParse("0.9")
			})

			when("DefaultProcessType", func() {
				when("is unset", func() {
					when("cmd starts with --", func() {
						it("creates a new direct process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"--", "user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: []string{"user-command"},
								Args:    []string{"user-arg1", "user-arg2"},
								Direct:  true,
							}, processCmpOpts...)
						})
					})

					when("cmd does not start with --", func() {
						it("creates a new shell process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: []string{"user-command"},
								Args:    []string{"user-arg1", "user-arg2"},
							}, processCmpOpts...)
						})
					})

					when("cmd is empty", func() {
						it("errors", func() {
							_, err := launcher.ProcessFor([]string{})
							h.AssertNotNil(t, err)
						})
					})
				})

				when("exists", func() {
					it.Before(func() {
						launcher.DefaultProcessType = "some-type"
					})

					it("appends cmd to the default process args", func() {
						proc, err := launcher.ProcessFor([]string{"user-arg1", "user-arg1"})
						h.AssertNil(t, err)
						h.AssertEq(t, proc, launch.Process{
							Type:        "some-type",
							Command:     []string{"some-command"},
							Args:        []string{"some-arg1", "some-arg2", "user-arg1", "user-arg1"},
							BuildpackID: "some-buildpack",
						}, processCmpOpts...)
					})
				})

				when("doesn't exist", func() {
					it.Before(func() {
						launcher.DefaultProcessType = "missing-type"
					})

					it("errors", func() {
						_, err := launcher.ProcessFor([]string{"user-arg1", "user-arg1"})
						h.AssertNotNil(t, err)
					})
				})
			})
		})

		when("Platform API < 0.4", func() {
			it.Before(func() {
				launcher.PlatformAPI = api.MustParse("0.3")
			})

			when("DefaultProcessType", func() {
				when("is unset", func() {
					when("cmd is empty", func() {
						it("errors", func() {
							_, err := launcher.ProcessFor([]string{})
							h.AssertNotNil(t, err)
						})
					})

					when("cmd contains a only a process type", func() {
						it("returns the process type", func() {
							proc, err := launcher.ProcessFor([]string{"other-type"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Type:        "other-type",
								Command:     []string{"other-command"},
								Args:        []string{"other-arg1", "other-arg2"},
								BuildpackID: "some-buildpack",
							}, processCmpOpts...)
						})
					})

					when("cmd starts with --", func() {
						it("creates a new direct process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"--", "user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: []string{"user-command"},
								Args:    []string{"user-arg1", "user-arg2"},
								Direct:  true,
							}, processCmpOpts...)
						})
					})

					when("cmd contains more than one arg and does not start with --", func() {
						it("creates a new shell process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: []string{"user-command"},
								Args:    []string{"user-arg1", "user-arg2"},
							}, processCmpOpts...)
						})
					})
				})

				when("exists", func() {
					it.Before(func() {
						launcher.DefaultProcessType = "some-type"
					})

					when("cmd is empty", func() {
						it("returns the default process type", func() {
							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Type:        "some-type",
								Command:     []string{"some-command"},
								Args:        []string{"some-arg1", "some-arg2"},
								BuildpackID: "some-buildpack",
							}, processCmpOpts...)
						})
					})

					when("cmd contains a only a process type", func() {
						it("returns the process type", func() {
							proc, err := launcher.ProcessFor([]string{"other-type"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Type:        "other-type",
								Command:     []string{"other-command"},
								Args:        []string{"other-arg1", "other-arg2"},
								BuildpackID: "some-buildpack",
							}, processCmpOpts...)
						})
					})

					when("cmd contains more than one arg and starts with --", func() {
						it("creates a new direct process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"--", "user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: []string{"user-command"},
								Args:    []string{"user-arg1", "user-arg2"},
								Direct:  true,
							}, processCmpOpts...)
						})
					})

					when("cmd contains more than one arg and does not start with --", func() {
						it("creates a new shell process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: []string{"user-command"},
								Args:    []string{"user-arg1", "user-arg2"},
							}, processCmpOpts...)
						})
					})
				})

				when("doesn't exist", func() {
					it.Before(func() {
						launcher.DefaultProcessType = "missing-type"
					})

					it("errors when cmd is empty", func() {
						_, err := launcher.ProcessFor([]string{})
						h.AssertNotNil(t, err)
					})
				})
			})
		})
	})
}
