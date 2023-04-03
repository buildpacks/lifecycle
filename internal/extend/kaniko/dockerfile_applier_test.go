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
		when(":ignorepaths", func() {
			it("adds them to the kaniko options", func() {
				opts := createOptions("someimage", "", extend.Dockerfile{
					Path: "/something",
					Args: []extend.Arg{{
						Name:  "arg1",
						Value: "val1",
					}},
				}, extend.Options{
					IgnorePaths: []string{"/path1", "/path/2"},
				})

				h.AssertEq(t, opts.IgnorePaths[0], "/path1")
				h.AssertEq(t, opts.IgnorePaths[1], "/path/2")
				h.AssertEq(t, len(opts.IgnorePaths), 2)
			})
		})

		when(":cacheTTL", func() {
			it("passes it to the kaniko options", func() {
				opts := createOptions("someimage", "", extend.Dockerfile{
					Path: "/something",
					Args: []extend.Arg{{
						Name:  "arg1",
						Value: "val1",
					}},
				}, extend.Options{
					CacheTTL: 7 * (24 * time.Hour),
				})

				h.AssertEq(t, opts.CacheOptions.CacheTTL, 7*(24*time.Hour))
			})
		})
	})
}
