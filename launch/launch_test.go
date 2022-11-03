package launch_test

import (
	"encoding/json"
	"fmt"
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
		when("MarhsalTOML", func() {
			it("command is array", func() {
				process := launch.Process{
					Type: "some-type",
					Command: launch.NewRawCommand([]string{"some-command", "some-command-arg1"}).
						WithPlatformAPI(api.Platform.Latest()),
					Args:             []string{"some-arg1"},
					Direct:           true,
					Default:          true,
					BuildpackID:      "some-buildpack-id",
					WorkingDirectory: "some-working-directory",
					PlatformAPI:      api.Platform.Latest(),
				}

				bytes, err := encoding.MarshalTOML(process)
				h.AssertNil(t, err)
				expected := `type = "some-type"
command = ["some-command", "some-command-arg1"]
args = ["some-arg1"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
				h.AssertEq(t, string(bytes), expected)
			})

			when("platform API < 0.10", func() {
				it("command is string", func() {
					process := launch.Process{
						Type:             "some-type",
						Command:          launch.NewRawCommand([]string{"some-command", "some-arg1"}),
						Args:             []string{"some-arg2"},
						Direct:           true,
						Default:          true,
						BuildpackID:      "some-buildpack-id",
						WorkingDirectory: "some-working-directory",
					}.WithPlatformAPI(api.MustParse("0.9"))

					bytes, err := encoding.MarshalTOML(process)
					h.AssertNil(t, err)
					expected := `type = "some-type"
command = "some-command"
args = ["some-arg1", "some-arg2"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
					fmt.Println("RENDERED: " + string(bytes))
					h.AssertEq(t, string(bytes), expected)
				})
			})
		})

		when("MarshalJSON", func() {
			it("command is array", func() {
				process := launch.Process{
					Type: "some-type",
					Command: launch.NewRawCommand([]string{"some-command", "some-command-arg1"}).
						WithPlatformAPI(api.Platform.Latest()),
					Args:             []string{"some-arg1"},
					Direct:           true,
					Default:          true,
					BuildpackID:      "some-buildpack-id",
					WorkingDirectory: "some-working-directory",
					PlatformAPI:      api.Platform.Latest(),
				}

				bytes, err := json.Marshal(process)
				h.AssertNil(t, err)
				expected := `{"type":"some-type","command":["some-command","some-command-arg1"],"args":["some-arg1"],"direct":true,"default":true,"buildpackID":"some-buildpack-id","working-dir":"some-working-directory"}`
				h.AssertEq(t, string(bytes), expected)
			})

			when("platform API < 0.10", func() {
				it("command is string", func() {
					process := launch.Process{
						Type:             "some-type",
						Command:          launch.NewRawCommand([]string{"some-command", "some-arg1"}),
						Args:             []string{"some-arg2"},
						Direct:           true,
						Default:          true,
						BuildpackID:      "some-buildpack-id",
						WorkingDirectory: "some-working-directory",
					}.WithPlatformAPI(api.MustParse("0.9"))

					bytes, err := json.Marshal(process)
					h.AssertNil(t, err)
					expected := `{"type":"some-type","command":"some-command","args":["some-arg1","some-arg2"],"direct":true,"default":true,"buildpackID":"some-buildpack-id","working-dir":"some-working-directory"}`
					h.AssertEq(t, string(bytes), expected)
				})
			})
		})

		when("UnmarshalTOML", func() {
			when("provided command as string", func() {
				it("populates a launch process", func() {
					data := `type = "some-type"
command = "some-command"
args = ["some-arg"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
					process := launch.Process{}
					_, err := toml.Decode(data, &process)
					h.AssertNil(t, err)
					if s := cmp.Diff([]launch.Process{process}, []launch.Process{
						{
							Type: "some-type",
							Command: launch.NewRawCommand([]string{"some-command"}).
								WithPlatformAPI(api.Platform.Latest()),
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
			})

			when("provided command as array", func() {
				it("populates a launch process", func() {
					data := `type = "some-type"
command = ["some-command", "some-command-arg"]
args = ["some-arg"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
					process := launch.Process{}
					_, err := toml.Decode(data, &process)
					h.AssertNil(t, err)
					if s := cmp.Diff([]launch.Process{process}, []launch.Process{
						{
							Type: "some-type",
							Command: launch.NewRawCommand([]string{"some-command", "some-command-arg"}).
								WithPlatformAPI(api.Platform.Latest()),
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
			})
		})
	})
}
