// +build acceptance

package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestAcceptance(t *testing.T) {
	spec.Run(t, "acceptance", testAcceptance, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAcceptance(t *testing.T, when spec.G, it spec.S) {

	when("All", func() {
		when("version flag is set", func() {
			for _, data := range [][]string{
				{"analyzer: only -version is present", "analyzer -version"},
				{"analyzer: other params are set", "analyzer -daemon -version some/image"},

				{"builder: only -version is present", "builder -version"},
				{"builder: other params are set", "builder -app=/some/dir -version some/image"},

				{"cacher: only -version is present", "cacher -version"},
				{"cacher: other params are set", "cacher -path=/some/dir -version"},

				{"detector: only -version is present", "detector -version"},
				{"detector: other params are set", "detector -app=/some/dir -version"},

				{"exporter: only -version is present", "exporter -version"},
				{"exporter: other params are set", "exporter -analyzed=/some/file -version some/image"},

				{"restorer: only -version is present", "restorer -version"},
				{"restorer: other params are set", "restorer -path=/some/dir -version"},
			} {
				desc := data[0]
				binary, args := parseCommand(data[1])

				when(desc, func() {
					it("only prints the version", func() {
						output, err := lifecycleCmd(t, binary, args...).CombinedOutput()
						if err != nil {
							t.Error(err)
						}

						h.AssertStringContains(t, string(output), "some-version")
					})
				})
			}
		})
	})
}

func lifecycleCmd(t *testing.T, name string, args ...string) *exec.Cmd {
	cmdArgs := append(
		[]string{
			"run",
			"-ldflags", "-X github.com/buildpack/lifecycle/cmd.buildVersion=some-version",
			"./cmd/" + name + "/main.go",
		}, args...,
	)

	wd, err := os.Getwd()
	h.AssertNil(t, err)

	cmd := exec.Command(
		"go",
		cmdArgs...,
	)
	cmd.Dir = filepath.Dir(wd)

	return cmd
}

func parseCommand(command string) (binary string, args []string) {
	parts := strings.Split(command, " ")
	return parts[0], parts[1:]
}
