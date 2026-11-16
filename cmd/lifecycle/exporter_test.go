package main

import (
	"context"
	"testing"

	"github.com/moby/moby/client"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExporter(t *testing.T) {
	spec.Run(t, "exporter", testExporter, spec.Report(report.Terminal{}))
}

// notFoundError is recognized as "not found" by the daemon image lookup.
type notFoundError struct{}

func (notFoundError) Error() string { return "no such image" }
func (notFoundError) NotFound()     {}

// emptyDaemon is a docker client whose daemon has no images.
type emptyDaemon struct {
	client.APIClient
}

func (emptyDaemon) ServerVersion(context.Context, client.ServerVersionOptions) (client.ServerVersionResult, error) {
	return client.ServerVersionResult{APIVersion: "1.48", Os: "linux", Arch: "amd64"}, nil
}

func (emptyDaemon) ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{}, notFoundError{}
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	when("#initDaemonAppImage", func() {
		when("the run image is not in the daemon", func() {
			it("fails with an error naming the run image", func() {
				e := &exportCmd{
					Platform: platform.NewPlatformFor("0.11"),
					docker:   emptyDaemon{},
				}
				e.RunImageRef = "some-run-image"
				e.OutputImageRef = "some-app-image"

				_, _, err := e.initDaemonAppImage(files.Analyzed{}, nil)

				h.AssertError(t, err, `run image "some-run-image" not found in daemon`)
			})
		})
	})
}
