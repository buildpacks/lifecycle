package acceptance

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"regexp"
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
				h.WithBash(fmt.Sprintf("%s -privileged-group privileged-group.toml -plan plan.toml; tar tvf /layers/example_stack/snapshot/snapshot.tgz", rootBuilderPath)),
			)

			h.AssertStringDoesNotContain(t, output, "file-to-ignore")
			h.AssertStringDoesNotContain(t, output, ".wh.sbin")
			h.AssertMatch(t, output, "bin/exe-to-snapshot")
			h.AssertMatch(t, output, "usr/bin/.wh.apt")
		})

		it("creates layers and snapshot", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

			output := h.DockerRun(t,
				rootBuilderImage,
				h.WithBash(fmt.Sprintf("%s -privileged-group privileged-group.toml -group group.toml -plan plan.toml; ls -al /layers/example_stack; ls -al /layers/example_stack/snapshot;ls -al /layers/example_user", rootBuilderPath)),
			)

			h.AssertMatch(t, output, "my-layer.toml")
			h.AssertMatch(t, output, "snapshot.tgz")
			h.AssertMatch(t, output, "snapshot.toml")
		})

		it("subsequent stack buildpacks only contain their file changes", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

			output := h.DockerRun(t,
				rootBuilderImage,
				h.WithBash(fmt.Sprintf("%s -privileged-group privileged-group.toml -group group.toml -plan plan.toml; tar tvf /layers/example_stack_b/snapshot/snapshot.tgz", rootBuilderPath)),
			)

			h.AssertMatch(t, output, "0/0 .+ bin/stack-b-file")
			if regexp.MustCompile("0/0 .+ bin/file-to-snapshot").MatchString(output) {
				t.Fatalf("Unexpected file in output: '%s'", output)
			}
		})

		when("there is an existing snapshot", func() {
			it("restored the snapshot prior to stack buildpack execution", func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

				output := h.DockerRun(t,
					rootBuilderImage,
					h.WithBash(fmt.Sprintf("%s -privileged-group privileged-group.toml -group group.toml -plan plan.toml; ls -al /layers/example_stack/snapshot;", rootBuilderPath)),
				)

				h.AssertMatch(t, output, "/bin/exe-to-snapshot restored from cache")
			})
		})
	})
}
