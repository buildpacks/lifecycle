package launch_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLaunch(t *testing.T) {
	spec.Run(t, "Launch", testLaunch, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testLaunch(t *testing.T, when spec.G, it spec.S) {
	when("Process", func() {
		when("MarshalTOML", func() {
			it("command is string", func() {
				process := launch.Process{
					Type:             "some-type",
					Command:          []string{"some-command"},
					Args:             []string{"some-arg"},
					Direct:           true,
					Default:          true,
					BuildpackID:      "some-buildpack-id",
					WorkingDirectory: "some-working-directory",
				}
				bytes, err := process.MarshalTOML()
				h.AssertNil(t, err)
				expected := `type = "some-type"
command = "some-command"
args = ["some-arg"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
				h.AssertEq(t, string(bytes), expected)
			})
		})

		when("UnmarshalTOML", func() {
			when("provided command as string", func() {
				it.Focus("populates a launch process", func() {
					data := `type = "some-type"
command = "some-command"
args = ["some-arg"]
direct = true
default = true
buildpack-id = "some-buildpack-id"
working-dir = "some-working-directory"
`
					process := &launch.Process{}
					h.AssertNil(t, process.UnmarshalTOML([]byte(data)))
					h.AssertEq(t, process, launch.Process{
						Type:             "some-type",
						Command:          []string{"some-command"},
						Args:             []string{"some-arg"},
						Direct:           true,
						Default:          true,
						BuildpackID:      "some-buildpack-id",
						WorkingDirectory: "some-working-directory",
					})
				})
			})

			when("provided command as array", func() {
				it("populates a launch process", func() {

				})
			})
		})
	})
}
