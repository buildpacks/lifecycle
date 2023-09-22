//go:build acceptance
// +build acceptance

package acceptance

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	rebaserTest  *PhaseTest
	rebaserPath  string
	rebaserImage string
)

func TestRebaser(t *testing.T) {
	testImageDockerContextFolder := filepath.Join("testdata", "rebaser")
	rebaserTest = NewPhaseTest(t, "rebaser", testImageDockerContextFolder)
	rebaserTest.Start(t, updateTOMLFixturesWithTestRegistry)
	defer rebaserTest.Stop(t)

	rebaserImage = rebaserTest.testImageRef
	rebaserPath = rebaserTest.containerBinaryPath

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-rebaser/"+platformAPI.String(), testRebaser(platformAPI.String()), spec.Sequential(), spec.Report(report.Terminal{}))
	}
}

func testRebaser(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("called with insecure registry flag", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "")
			})
			it("should do an http request", func() {
				insecureRegistry := "host.docker.internal"
				rebaserOutputImageName := insecureRegistry + "/bar"
				_, _, err := h.DockerRunWithError(t,
					rebaserImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_INSECURE_REGISTRIES="+insecureRegistry,
					),
					h.WithArgs(ctrPath(rebaserPath), rebaserOutputImageName),
				)

				h.AssertStringContains(t, err.Error(), "http://host.docker.internal")
			})
		})
	}
}
