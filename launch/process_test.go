package launch_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestProcess(t *testing.T) {
	spec.Run(t, "Process", testProcess, spec.Report(report.Terminal{}))
}

func testProcess(t *testing.T, when spec.G, it spec.S) {
	var (
		launcher *launch.Launcher
	)
	it.Before(func() {
		launcher = &launch.Launcher{Processes: []launch.Process{
			{
				Type:        "some-type",
				Command:     "some-command",
				Args:        []string{"some-arg1", "some-arg2"},
				BuildpackID: "some-buildpack",
			},
			{
				Type:        "other-type",
				Command:     "other-command",
				Args:        []string{"other-arg1", "other-arg2"},
				BuildpackID: "some-buildpack",
			},
		}}
	})

	when("ProcessFor", func() {
		when("Platform API >= 0.4", func() {
			it.Before(func() {
				launcher.PlatformAPI = api.MustParse("0.4")
			})

			when("DefaultProcessType", func() {
				when("is unset", func() {
					when("cmd starts with --", func() {
						it("creates a new direct process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"--", "user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: "user-command",
								Args:    []string{"user-arg1", "user-arg2"},
								Direct:  true,
							})
						})
					})

					when("cmd does not starts with --", func() {
						it("creates a new shell process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: "user-command",
								Args:    []string{"user-arg1", "user-arg2"},
							})
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
							Command:     "some-command",
							Args:        []string{"some-arg1", "some-arg2", "user-arg1", "user-arg1"},
							BuildpackID: "some-buildpack",
						})
					})
				})

				when("doesn't exit", func() {
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
								Command:     "other-command",
								Args:        []string{"other-arg1", "other-arg2"},
								BuildpackID: "some-buildpack",
							})
						})
					})

					when("cmd starts with --", func() {
						it("creates a new direct process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"--", "user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: "user-command",
								Args:    []string{"user-arg1", "user-arg2"},
								Direct:  true,
							})
						})
					})

					when("cmd contains more than one arg and does not start with --", func() {
						it("creates a new shell process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: "user-command",
								Args:    []string{"user-arg1", "user-arg2"},
							})
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
								Command:     "some-command",
								Args:        []string{"some-arg1", "some-arg2"},
								BuildpackID: "some-buildpack",
							})
						})
					})

					when("cmd contains a only a process type", func() {
						it("returns the process type", func() {
							proc, err := launcher.ProcessFor([]string{"other-type"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Type:        "other-type",
								Command:     "other-command",
								Args:        []string{"other-arg1", "other-arg2"},
								BuildpackID: "some-buildpack",
							})
						})
					})

					when("cmd contains more than one arg and starts with --", func() {
						it("creates a new direct process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"--", "user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: "user-command",
								Args:    []string{"user-arg1", "user-arg2"},
								Direct:  true,
							})
						})
					})

					when("cmd contains more than one arg and does not start with --", func() {
						it("creates a new shell process from cmd", func() {
							proc, err := launcher.ProcessFor([]string{"user-command", "user-arg1", "user-arg2"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Command: "user-command",
								Args:    []string{"user-arg1", "user-arg2"},
							})
						})
					})
				})

				when("doesn't exit", func() {
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
