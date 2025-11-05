//go:build unix

package launch_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestUmask(t *testing.T) {
	spec.Run(t, "Umask", testUmask, spec.Report(report.Terminal{}))
}

func testUmask(t *testing.T, when spec.G, it spec.S) {
	when("UMASK is set", func() {
		it("parses octal umask values", func() {
			tests := []struct {
				name     string
				umask    string
				expected int
			}{
				{"standard user umask", "0002", 2},
				{"restrictive umask", "0077", 63},
				{"permissive umask", "0000", 0},
				{"three digit umask", "022", 18},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					environ := env.NewLaunchEnv([]string{"UMASK=" + tt.umask}, "", "")

					var called int
					spy := func(m int) int {
						called = m
						return 0
					}

					err := launch.SetUmaskWith(environ, spy)

					h.AssertNil(t, err)
					h.AssertEq(t, called, tt.expected)
				})
			}
		})

		it("returns error for invalid umask", func() {
			environ := env.NewLaunchEnv([]string{"UMASK=invalid"}, "", "")

			err := launch.SetUmaskWith(environ, func(int) int { return 0 })

			h.AssertNotNil(t, err)
		})
	})

	when("UMASK is unset", func() {
		it("does not call umask function", func() {
			environ := env.NewLaunchEnv([]string{}, "", "")

			called := false
			spy := func(_ int) int {
				called = true
				return 0
			}

			err := launch.SetUmaskWith(environ, spy)

			h.AssertNil(t, err)
			h.AssertEq(t, called, false)
		})
	})
}
