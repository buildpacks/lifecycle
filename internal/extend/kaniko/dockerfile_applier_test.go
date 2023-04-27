package kaniko

import (
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/internal/extend"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDockerApplier(t *testing.T) {
	spec.Run(t, "DockerApplier", testDockerApplier, spec.Report(report.Terminal{}))
}

func testDockerApplier(t *testing.T, when spec.G, it spec.S) {
	when("#createOptions", func() {
		when("ignore paths", func() {
			it("adds them to kaniko options", func() {
				opts := createOptions(
					"some-image-ref",
					extend.Dockerfile{
						Path: "/something",
						Args: []extend.Arg{{
							Name:  "arg1",
							Value: "val1",
						}},
					},
					extend.Options{
						IgnorePaths: []string{"/path1", "/path/2"},
					},
				)

				h.AssertEq(t, opts.IgnorePaths[0], "/path1")
				h.AssertEq(t, opts.IgnorePaths[1], "/path/2")
				h.AssertEq(t, len(opts.IgnorePaths), 2)
			})
		})

		when("cache TTL", func() {
			it("adds it to kaniko options", func() {
				opts := createOptions(
					"some-image-ref",
					extend.Dockerfile{
						Path: "/something",
						Args: []extend.Arg{{
							Name:  "arg1",
							Value: "val1",
						}},
					},
					extend.Options{
						CacheTTL: 7 * (24 * time.Hour),
					},
				)

				h.AssertEq(t, opts.CacheOptions.CacheTTL, 7*(24*time.Hour))
			})
		})

		when("cache dir", func() {
			// If we provide cache directory as an option, kaniko looks there for the base image as a tarball;
			// however the base image is in OCI layout format, so we fail to initialize the base image,
			// causing a confusing log message to be emitted by kaniko.
			it("does not add it to kaniko options", func() {
				opts := createOptions(
					"some-image-ref",
					extend.Dockerfile{
						Path: "/something",
						Args: []extend.Arg{{
							Name:  "arg1",
							Value: "val1",
						}},
					},
					extend.Options{},
				)

				h.AssertEq(t, opts.CacheOptions.CacheDir, "")
			})
		})
	})
}
