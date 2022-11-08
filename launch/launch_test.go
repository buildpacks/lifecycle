package launch_test

import (
	"encoding/json"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLaunch(t *testing.T) {
	spec.Run(t, "Launch", testLaunch, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testLaunch(t *testing.T, when spec.G, it spec.S) {
	when("Process", func() {
		when("MarshalTOML", func() {
			it("output command is array", func() {
				process := launch.Process{
					Type:             "some-type",
					Command:          launch.NewRawCommand([]string{"some-command", "some-command-arg"}),
					Args:             []string{"some-arg"},
					Direct:           true,
					Default:          true,
					BuildpackID:      "some-buildpack-id",
					WorkingDirectory: "some-working-directory",
				}.WithPlatformAPI(api.Platform.Latest())

				bytes, err := encoding.MarshalTOML(process)
				h.AssertNil(t, err)
				expected := `type = "some-type"
command = ["some-command", "some-command-arg"]
args = ["some-arg"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
				h.AssertEq(t, string(bytes), expected)
			})

			it("handles special characters", func() {
				process := launch.Process{
					Type:        "some-type",
					Command:     launch.RawCommand{Entries: []string{`\r`}},
					Args:        []string{`\r`},
					BuildpackID: "some-buildpack-id",
				}.WithPlatformAPI(api.Platform.Latest())

				bytes, err := encoding.MarshalTOML(process)
				h.AssertNil(t, err)
				expected := `type = "some-type"
command = ["\\r"]
args = ["\\r"]
direct = false
buildpack-id = "some-buildpack-id"
`
				h.AssertEq(t, string(bytes), expected)
			})

			when("platform API < 0.10", func() {
				it("output command is string", func() {
					process := launch.Process{
						Type:             "some-type",
						Command:          launch.NewRawCommand([]string{"some-command", "some-command-arg"}),
						Args:             []string{"some-arg"},
						Direct:           true,
						Default:          true,
						BuildpackID:      "some-buildpack-id",
						WorkingDirectory: "some-working-directory",
					}.WithPlatformAPI(api.MustParse("0.9"))

					bytes, err := encoding.MarshalTOML(process)
					h.AssertNil(t, err)
					expected := `type = "some-type"
command = "some-command"
args = ["some-command-arg", "some-arg"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
					h.AssertEq(t, string(bytes), expected)
				})

				it("handles special characters", func() {
					process := launch.Process{
						Type:        "some-type",
						Command:     launch.RawCommand{Entries: []string{`\r`}},
						Args:        []string{`\r`},
						BuildpackID: "some-buildpack-id",
					}.WithPlatformAPI(api.MustParse("0.9"))

					bytes, err := encoding.MarshalTOML(process)
					h.AssertNil(t, err)
					expected := `type = "some-type"
command = "\\r"
args = ["\\r"]
direct = false
buildpack-id = "some-buildpack-id"
`
					h.AssertEq(t, string(bytes), expected)
				})
			})
		})

		when("MarshalJSON", func() {
			it("output command is array", func() {
				process := launch.Process{
					Type:             "some-type",
					Command:          launch.NewRawCommand([]string{"some-command", "some-command-arg"}),
					Args:             []string{"some-arg"},
					Direct:           true,
					Default:          true,
					BuildpackID:      "some-buildpack-id",
					WorkingDirectory: "some-working-directory",
				}.WithPlatformAPI(api.Platform.Latest())

				bytes, err := json.Marshal(process)
				h.AssertNil(t, err)
				expected := `{"type":"some-type","command":["some-command","some-command-arg"],"args":["some-arg"],"direct":true,"default":true,"buildpackID":"some-buildpack-id","working-dir":"some-working-directory"}`
				h.AssertEq(t, string(bytes), expected)
			})

			it("handles special characters", func() {
				process := launch.Process{
					Type:        "some-type",
					Command:     launch.RawCommand{Entries: []string{`\r`}},
					Args:        []string{`\r`},
					BuildpackID: "some-buildpack-id",
				}.WithPlatformAPI(api.Platform.Latest())

				bytes, err := json.Marshal(process)
				h.AssertNil(t, err)
				expected := `{"type":"some-type","command":["\\r"],"args":["\\r"],"direct":false,"buildpackID":"some-buildpack-id"}`
				h.AssertEq(t, string(bytes), expected)
			})

			when("platform API < 0.10", func() {
				it("output command is string", func() {
					process := launch.Process{
						Type:             "some-type",
						Command:          launch.NewRawCommand([]string{"some-command", "some-command-arg"}),
						Args:             []string{"some-arg"},
						Direct:           true,
						Default:          true,
						BuildpackID:      "some-buildpack-id",
						WorkingDirectory: "some-working-directory",
					}.WithPlatformAPI(api.MustParse("0.9"))

					bytes, err := json.Marshal(process)
					h.AssertNil(t, err)
					expected := `{"type":"some-type","command":"some-command","args":["some-command-arg","some-arg"],"direct":true,"default":true,"buildpackID":"some-buildpack-id","working-dir":"some-working-directory"}`
					h.AssertEq(t, string(bytes), expected)
				})

				it("handles special characters", func() {
					process := launch.Process{
						Type:        "some-type",
						Command:     launch.RawCommand{Entries: []string{`\r`}},
						Args:        []string{`\r`},
						BuildpackID: "some-buildpack-id",
					}.WithPlatformAPI(api.MustParse("0.9"))

					bytes, err := json.Marshal(process)
					h.AssertNil(t, err)
					expected := `{"type":"some-type","command":"\\r","args":["\\r"],"direct":false,"buildpackID":"some-buildpack-id"}`
					h.AssertEq(t, string(bytes), expected)
				})
			})
		})

		when("UnmarshalTOML", func() {
			when("input command is string", func() {
				it("populates a launch process", func() {
					data := `type = "some-type"
command = "some-command"
args = ["some-arg"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
					var process launch.Process
					_, err := toml.Decode(data, &process)
					h.AssertNil(t, err)
					if s := cmp.Diff([]launch.Process{process}, []launch.Process{
						{
							Type:             "some-type",
							Command:          launch.NewRawCommand([]string{"some-command"}),
							Args:             []string{"some-arg"},
							Direct:           true,
							Default:          true,
							BuildpackID:      "some-buildpack-id",
							WorkingDirectory: "some-working-directory",
						},
					}, processCmpOpts...); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}
				})

				it("handles special characters", func() {
					data := `type = "some-type"
command = "\\r"
args = ["\\r"]
direct = false
buildpack-id = "some-buildpack-id"
`
					var process launch.Process

					err := toml.Unmarshal([]byte(data), &process)
					h.AssertNil(t, err)
					expected := launch.Process{
						Type:        "some-type",
						Command:     launch.RawCommand{Entries: []string{`\r`}},
						Args:        []string{`\r`},
						BuildpackID: "some-buildpack-id",
					}
					h.AssertEq(t, process, expected, processCmpOpts...)
				})
			})

			when("input command is array", func() {
				it("populates a launch process", func() {
					data := `type = "some-type"
command = ["some-command", "some-command-arg"]
args = ["some-arg"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
					var process launch.Process
					_, err := toml.Decode(data, &process)
					h.AssertNil(t, err)
					if s := cmp.Diff([]launch.Process{process}, []launch.Process{
						{
							Type:             "some-type",
							Command:          launch.NewRawCommand([]string{"some-command", "some-command-arg"}),
							Args:             []string{"some-arg"},
							Direct:           true,
							Default:          true,
							BuildpackID:      "some-buildpack-id",
							WorkingDirectory: "some-working-directory",
						},
					}, processCmpOpts...); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}
				})

				it("handles special characters", func() {
					data := `type = "some-type"
command = ["\\r"]
args = ["\\r"]
direct = false
buildpack-id = "some-buildpack-id"
`
					var process launch.Process
					err := toml.Unmarshal([]byte(data), &process)
					h.AssertNil(t, err)
					expected := launch.Process{
						Type:        "some-type",
						Command:     launch.RawCommand{Entries: []string{`\r`}},
						Args:        []string{`\r`},
						BuildpackID: "some-buildpack-id",
					}
					h.AssertEq(t, process, expected, processCmpOpts...)
				})
			})
		})

		when.Pend("UnmarshalJSON", func() {
			when("input command is string", func() {
				it("populates a launch process", func() {
					data := `{"type":"some-type","command":"some-command","args":["some-arg"],"direct":true,"default":true,"buildpackID":"some-buildpack-id","working-dir":"some-working-directory"}`
					var process launch.Process
					err := json.Unmarshal([]byte(data), &process)
					h.AssertNil(t, err)
					if s := cmp.Diff([]launch.Process{process}, []launch.Process{
						{
							Type:             "some-type",
							Command:          launch.NewRawCommand([]string{"some-command"}),
							Args:             []string{"some-arg"},
							Direct:           true,
							Default:          true,
							BuildpackID:      "some-buildpack-id",
							WorkingDirectory: "some-working-directory",
						},
					}, processCmpOpts...); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}
				})

				it("handles special characters", func() {
					data := `{"type":"some-type","command":"\\r","args":["\\r"],"direct":false,"buildpackID":"some-buildpack-id"}`
					var process launch.Process
					err := json.Unmarshal([]byte(data), &process)
					h.AssertNil(t, err)
					expected := launch.Process{
						Type:        "some-type",
						Command:     launch.RawCommand{Entries: []string{`\\r`}}, // TODO: check
						Args:        []string{`\r`},
						BuildpackID: "some-buildpack-id",
					}
					h.AssertEq(t, process, expected, processCmpOpts...)
				})
			})

			when("input command is array", func() {
				it("populates a launch process", func() {
					data := `{"type":"some-type","command":["some-command","some-command-arg"],"args":["some-arg"],"direct":true,"default":true,"buildpackID":"some-buildpack-id","working-dir":"some-working-directory"}`
					var process launch.Process
					err := json.Unmarshal([]byte(data), &process)
					h.AssertNil(t, err)
					if s := cmp.Diff([]launch.Process{process}, []launch.Process{
						{
							Type:             "some-type",
							Command:          launch.NewRawCommand([]string{"some-command", "some-command-arg"}), // TODO: fix
							Args:             []string{"some-arg"},
							Direct:           true,
							Default:          true,
							BuildpackID:      "some-buildpack-id",
							WorkingDirectory: "some-working-directory",
						},
					}, processCmpOpts...); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}
				})

				it("handles special characters", func() {
					data := `{"type":"some-type","command":["\\r"],"args":["\\r"],"direct":false,"buildpackID":"some-buildpack-id"}`
					var process launch.Process
					err := json.Unmarshal([]byte(data), &process)
					h.AssertNil(t, err)
					expected := launch.Process{
						Type:        "some-type",
						Command:     launch.RawCommand{Entries: []string{`\r`}}, // TODO: fix
						Args:        []string{`\r`},
						BuildpackID: "some-buildpack-id",
					}
					h.AssertEq(t, process, expected, processCmpOpts...)
				})
			})
		})
	})
}
