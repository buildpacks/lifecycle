package acceptance

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/acceptance/variables"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	rootBuilderBinaryDir     = filepath.Join("testdata", "builder", "image", "container", "cnb", "lifecycle")
	rootBuilderDockerContext = filepath.Join("testdata", "builder", "image")
	rootBuilderImage         = "lifecycle/acceptance/builder"
	rootBuilderPath          = "/cnb/lifecycle/builder"
)

func TestStackBuilder(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "These tests need to be adapted to work on Windows")
	rand.Seed(time.Now().UTC().UnixNano())

	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)
	daemonOS = info.OSType

	// Setup test container

	h.MakeAndCopyLifecycle(t, daemonOS, rootBuilderBinaryDir)
	h.DockerBuild(t,
		rootBuilderImage,
		rootBuilderDockerContext,
		h.WithFlags("-f", filepath.Join(rootBuilderDockerContext, variables.DockerfileName)),
	)
	defer h.DockerImageRemove(t, rootBuilderImage)

	spec.Run(t, "acceptance-builder", testStackBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testStackBuilder(t *testing.T, when spec.G, it spec.S) {
	when("called", func() {
		it("creates a snapshot", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

			output := h.DockerRun(t,
				rootBuilderImage,
				h.WithBash(fmt.Sprintf("%s -stack-group stack-group.toml -plan plan.toml; tar tvf /layers/example_stack.tgz", rootBuilderPath)),
			)

			h.AssertStringDoesNotContain(t, output, "file-to-ignore")
			h.AssertStringDoesNotContain(t, output, ".wh.sbin")
			h.AssertMatch(t, output, "bin/exe-to-snapshot")
			// TODO verify timestamp is "1970-01-01 00:00"
		})

		it("creates layers and snapshot", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

			output := h.DockerRun(t,
				rootBuilderImage,
				h.WithBash(fmt.Sprintf("%s -stack-group stack-group.toml -group group.toml -plan plan.toml; ls -al /layers/; ls -al /layers/example_user", rootBuilderPath)),
			)

			h.AssertMatch(t, output, "example_stack.tgz")
			h.AssertMatch(t, output, "my-layer.toml")
		})
	})
}
