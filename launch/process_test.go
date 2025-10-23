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

// PlatformAPI should be ignored because it is not always set in these tests
var processCmpOpts = []cmp.Option{
	cmpopts.IgnoreFields(launch.Process{}, "PlatformAPI"),
	cmpopts.IgnoreFields(launch.RawCommand{}, "PlatformAPI"),
}

func testProcess(t *testing.T, when spec.G, it spec.S) {
	var (
		launcher *launch.Launcher
	)
	it.Before(func() {
		launcher = &launch.Launcher{
			Buildpacks: []launch.Buildpack{
				{ID: "some-buildpack", API: "0.8"},
				{ID: "some-newer-buildpack", API: "0.9"},
			},
			Processes: []launch.Process{
				{
					Type:        "some-type",
					Command:     launch.NewRawCommand([]string{"some-command"}),
					Args:        []string{"some-arg1", "some-arg2"},
					BuildpackID: "some-buildpack",
				},
				{
					Type:        "other-type",
					Command:     launch.NewRawCommand([]string{"other-command"}),
					Args:        []string{"other-arg1", "other-arg2"},
					BuildpackID: "some-buildpack",
				},
				{
					Type:        "type-with-always-and-overridable-args",
					Command:     launch.NewRawCommand([]string{"some-command", "always-arg"}),
					Args:        []string{"overridable-arg"},
					BuildpackID: "some-newer-buildpack",
				},
				{
					Type:        "type-with-overridable-arg",
					Command:     launch.NewRawCommand([]string{"some-command"}),
					Args:        []string{"overridable-arg"},
					BuildpackID: "some-newer-buildpack",
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
							Command: launch.NewRawCommand([]string{"user-command"}),
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
							Command: launch.NewRawCommand([]string{"user-command"}),
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
				when("process' 'command' has args", func() {
					it.Before(func() {
						launcher.DefaultProcessType = "type-with-always-and-overridable-args"
					})

					it("provides command args and replaces process args with cmd", func() {
						proc, err := launcher.ProcessFor([]string{"user-arg1", "user-arg2"})
						h.AssertNil(t, err)
						h.AssertEq(t, proc, launch.Process{
							Type:        "type-with-always-and-overridable-args",
							Command:     launch.NewRawCommand([]string{"some-command"}),
							Args:        []string{"always-arg", "user-arg1", "user-arg2"},
							BuildpackID: "some-newer-buildpack",
						}, processCmpOpts...)
					})

					when("cmd is empty", func() {
						it("provides command args and process args", func() {
							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Type:        "type-with-always-and-overridable-args",
								Command:     launch.NewRawCommand([]string{"some-command"}),
								Args:        []string{"always-arg", "overridable-arg"},
								BuildpackID: "some-newer-buildpack",
							}, processCmpOpts...)
						})
					})
				})

				when("process' 'command' does not have args", func() {
					when("newer buildpack with API >= 0.9", func() {
						it.Before(func() {
							launcher.DefaultProcessType = "type-with-overridable-arg"
						})

						it("replaces process args with cmd", func() {
							proc, err := launcher.ProcessFor([]string{"user-arg1", "user-arg1"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Type:        "type-with-overridable-arg",
								Command:     launch.NewRawCommand([]string{"some-command"}),
								Args:        []string{"user-arg1", "user-arg1"},
								BuildpackID: "some-newer-buildpack",
							}, processCmpOpts...)
						})

						when("cmd is empty", func() {
							it("provides process args", func() {
								proc, err := launcher.ProcessFor([]string{})
								h.AssertNil(t, err)
								h.AssertEq(t, proc, launch.Process{
									Type:        "type-with-overridable-arg",
									Command:     launch.NewRawCommand([]string{"some-command"}),
									Args:        []string{"overridable-arg"},
									BuildpackID: "some-newer-buildpack",
								}, processCmpOpts...)
							})
						})
					})

					when("older buildpack with API < 0.9", func() {
						it.Before(func() {
							launcher.DefaultProcessType = "some-type"
						})

						it("appends cmd to process args", func() {
							proc, err := launcher.ProcessFor([]string{"user-arg1", "user-arg1"})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Type:        "some-type",
								Command:     launch.NewRawCommand([]string{"some-command"}),
								Args:        []string{"some-arg1", "some-arg2", "user-arg1", "user-arg1"},
								BuildpackID: "some-buildpack",
							}, processCmpOpts...)
						})
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
								Command: launch.NewRawCommand([]string{"user-command"}),
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
								Command: launch.NewRawCommand([]string{"user-command"}),
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
						launcher.DefaultProcessType = "type-with-overridable-arg"
					})

					it("appends cmd to process args", func() {
						proc, err := launcher.ProcessFor([]string{"user-arg1", "user-arg1"})
						h.AssertNil(t, err)
						h.AssertEq(t, proc, launch.Process{
							Type:        "type-with-overridable-arg",
							Command:     launch.NewRawCommand([]string{"some-command"}),
							Args:        []string{"overridable-arg", "user-arg1", "user-arg1"},
							BuildpackID: "some-newer-buildpack",
						}, processCmpOpts...)
					})

					when("cmd is empty", func() {
						it("provides process args", func() {
							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc, launch.Process{
								Type:        "type-with-overridable-arg",
								Command:     launch.NewRawCommand([]string{"some-command"}),
								Args:        []string{"overridable-arg"},
								BuildpackID: "some-newer-buildpack",
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

			when("execution environment filtering", func() {
				it.Before(func() {
					launcher.PlatformAPI = api.MustParse("0.15") // Enable execution environment filtering
					launcher.ExecEnv = "test"
					launcher.DefaultProcessType = "web"
					launcher.Processes = []launch.Process{
						{
							Type:        "web",
							Command:     launch.NewRawCommand([]string{"web-server"}),
							Args:        []string{"--port", "8080"},
							BuildpackID: "some-buildpack",
						},
						{
							Type:        "prod-only",
							Command:     launch.NewRawCommand([]string{"prod-command"}),
							Args:        []string{},
							BuildpackID: "some-buildpack",
							ExecEnv:     []string{"production"},
						},
						{
							Type:        "test-only",
							Command:     launch.NewRawCommand([]string{"test-command"}),
							Args:        []string{},
							BuildpackID: "some-buildpack",
							ExecEnv:     []string{"test"},
						},
						{
							Type:        "multi-env",
							Command:     launch.NewRawCommand([]string{"multi-command"}),
							Args:        []string{},
							BuildpackID: "some-buildpack",
							ExecEnv:     []string{"production", "test"},
						},
						{
							Type:        "wildcard",
							Command:     launch.NewRawCommand([]string{"wildcard-command"}),
							Args:        []string{},
							BuildpackID: "some-buildpack",
							ExecEnv:     []string{"*"},
						},
					}
				})

				when("Platform API < 0.15", func() {
					it("ignores execution environment and returns any process", func() {
						launcher.PlatformAPI = api.MustParse("0.14")
						launcher.ExecEnv = "test"
						launcher.DefaultProcessType = "prod-only" // This would normally be filtered out

						proc, err := launcher.ProcessFor([]string{})
						h.AssertNil(t, err)
						h.AssertEq(t, proc.Type, "prod-only")
					})
				})

				when("Platform API >= 0.15", func() {
					when("process has no exec-env specified", func() {
						it("returns the process (applies to all execution environments)", func() {
							launcher.DefaultProcessType = "web"

							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc.Type, "web")
						})
					})

					when("process supports wildcard execution environment", func() {
						it("returns the process", func() {
							launcher.DefaultProcessType = "wildcard"

							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc.Type, "wildcard")
						})
					})

					when("process supports the current execution environment", func() {
						it("returns the process", func() {
							launcher.DefaultProcessType = "test-only"

							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc.Type, "test-only")
						})

						it("returns the process when multiple environments are supported", func() {
							launcher.DefaultProcessType = "multi-env"

							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc.Type, "multi-env")
						})
					})

					when("process does not support the current execution environment", func() {
						it("returns an error", func() {
							launcher.DefaultProcessType = "prod-only"

							_, err := launcher.ProcessFor([]string{})
							h.AssertNotNil(t, err)
							h.AssertStringContains(t, err.Error(), "prod-only")
						})
					})

					when("multiple processes exist with same type but different exec-env", func() {
						it.Before(func() {
							launcher.Processes = append(launcher.Processes, []launch.Process{
								{
									Type:        "duplicate",
									Command:     launch.NewRawCommand([]string{"prod-version"}),
									BuildpackID: "some-buildpack",
									ExecEnv:     []string{"production"},
								},
								{
									Type:        "duplicate",
									Command:     launch.NewRawCommand([]string{"test-version"}),
									BuildpackID: "some-buildpack",
									ExecEnv:     []string{"test"},
								},
							}...)
						})

						it("returns the first process that supports the current execution environment", func() {
							launcher.DefaultProcessType = "duplicate"

							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc.Type, "duplicate")
							h.AssertEq(t, proc.Command.Entries, []string{"test-version"})
						})
					})

					when("execution environment is empty", func() {
						it("only matches processes with no exec-env specified", func() {
							launcher.ExecEnv = ""
							launcher.DefaultProcessType = "web" // This has no ExecEnv, so should work

							proc, err := launcher.ProcessFor([]string{})
							h.AssertNil(t, err)
							h.AssertEq(t, proc.Type, "web")
						})

						it("does not match processes with specific exec-env requirements", func() {
							launcher.ExecEnv = ""
							launcher.DefaultProcessType = "prod-only" // This has ExecEnv: ["production"]

							_, err := launcher.ProcessFor([]string{})
							h.AssertNotNil(t, err)
							h.AssertStringContains(t, err.Error(), "prod-only")
						})
					})
				})
			})
		})
	})
}
