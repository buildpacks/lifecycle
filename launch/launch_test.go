package launch_test

import (
	"encoding/json"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLaunch(t *testing.T) {
	t.Run("Process", func(t *testing.T) {
		t.Run("MarshalTOML", func(t *testing.T) {
			t.Run("output command is array", func(t *testing.T) {
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
			t.Run("handles special characters", func(t *testing.T) {
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
			t.Run("platform API < 0.10", func(t *testing.T) {
				t.Run("output command is string", func(t *testing.T) {
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
				t.Run("handles special characters", func(t *testing.T) {
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
		t.Run("MarshalJSON", func(t *testing.T) {
			t.Run("output command is array", func(t *testing.T) {
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
			t.Run("handles special characters", func(t *testing.T) {
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
			t.Run("platform API < 0.10", func(t *testing.T) {
				t.Run("output command is string", func(t *testing.T) {
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
				t.Run("handles special characters", func(t *testing.T) {
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
		t.Run("UnmarshalTOML", func(t *testing.T) {
			t.Run("input command is string", func(t *testing.T) {
				t.Run("populates a launch process", func(t *testing.T) {
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
				t.Run("handles special characters", func(t *testing.T) {
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
			t.Run("input command is array", func(t *testing.T) {
				t.Run("populates a launch process", func(t *testing.T) {
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
				t.Run("handles special characters", func(t *testing.T) {
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
		t.Run("UnmarshalJSON", func(t *testing.T) {
			t.Run("input command is string", func(t *testing.T) {
				t.Run("populates a launch process", func(t *testing.T) {
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
				t.Run("handles special characters", func(t *testing.T) {
					data := `{"type":"some-type","command":"\\r","args":["\\r"],"direct":false,"buildpackID":"some-buildpack-id"}`
					var process launch.Process
					err := json.Unmarshal([]byte(data), &process)
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
			t.Run("input command is array", func(t *testing.T) {
				t.Run("populates a launch process", func(t *testing.T) {
					data := `{"type":"some-type","command":["some-command","some-command-arg"],"args":["some-arg"],"direct":true,"default":true,"buildpackID":"some-buildpack-id","working-dir":"some-working-directory"}`
					var process launch.Process
					err := json.Unmarshal([]byte(data), &process)
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
				t.Run("handles special characters", func(t *testing.T) {
					data := `{"type":"some-type","command":["\\r"],"args":["\\r"],"direct":false,"buildpackID":"some-buildpack-id"}`
					var process launch.Process
					err := json.Unmarshal([]byte(data), &process)
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
	})
}
