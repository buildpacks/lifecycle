package kaniko

import (
	"testing"
	"time"

	"github.com/buildpacks/lifecycle/internal/extend"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDockerApplier(t *testing.T) {
	t.Run("#createOptions", func(t *testing.T) {
		t.Run("ignore paths", func(t *testing.T) {
			t.Run("adds them to kaniko options", func(t *testing.T) {
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
		t.Run("cache TTL", func(t *testing.T) {
			t.Run("adds it to kaniko options", func(t *testing.T) {
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
		t.Run("cache dir", func(t *testing.
			// If we provide cache directory as an option, kaniko looks there for the base image as a tarball;
			// however the base image is in OCI layout format, so we fail to initialize the base image,
			// causing a confusing log message to be emitted by kaniko.
			T) {
			t.Run("does not add it to kaniko options", func(t *testing.T) {
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
