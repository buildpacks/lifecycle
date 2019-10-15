package compat_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/compat"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestCompat(t *testing.T) {
	spec.Run(t, "testCompat", testCompat, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCompat(t *testing.T, when spec.G, it spec.S) {
	when("#ReadOrder", func() {
		when("order toml is v1", func() {
			it("should parse groups", func() {
				order, err := compat.ReadOrder(filepath.Join("testdata", "v1.order.toml"), filepath.Join("testdata", "buildpacks"))
				h.AssertNil(t, err)
				h.AssertEq(t, order, lifecycle.BuildpackOrder{
					{
						Group: []lifecycle.Buildpack{
							{
								ID:       "buildpack.a",
								Version:  "buildpack.a.v1",
								Optional: false,
							},
							{
								ID:       "buildpack.b",
								Version:  "buildpack.b.v1",
								Optional: true,
							},
						},
					},
					{
						Group: []lifecycle.Buildpack{
							{
								ID:       "buildpack.c",
								Version:  "buildpack.c.v1",
								Optional: false,
							},
						},
					},
				})
			})

			when("buildpack version latest", func() {
				when("single matching buildpack", func() {
					it("should resolve version", func() {
						order, err := compat.ReadOrder(filepath.Join("testdata", "v1.order.single.toml"), filepath.Join("testdata", "buildpacks"))
						h.AssertNil(t, err)
						h.AssertEq(t, order, lifecycle.BuildpackOrder{
							{
								Group: []lifecycle.Buildpack{
									{
										ID:       "buildpack.single",
										Version:  "buildpack.single.v1",
										Optional: false,
									},
								},
							},
						})
					})
				})

				when("multiple matching buildpacks", func() {
					it("should error out", func() {
						_, err := compat.ReadOrder(filepath.Join("testdata", "v1.order.dup.toml"), filepath.Join("testdata", "buildpacks"))
						h.AssertError(t, err, "too many buildpacks with matching ID 'buildpack.dup'")
					})
				})

				when("no matching buildpacks", func() {
					it("should error out", func() {
						_, err := compat.ReadOrder(filepath.Join("testdata", "v1.order.nonexistent.toml"), filepath.Join("testdata", "buildpacks"))
						h.AssertError(t, err, "no buildpacks with matching ID 'buildpack.nonexistent'")
					})
				})
			})
		})

		when("order toml is not v1 format", func() {
			it("should return empty order", func() {
				order, err := compat.ReadOrder(filepath.Join("testdata", "v2.order.toml"), "")
				h.AssertNil(t, err)
				h.AssertEq(t, len(order), 0)
			})
		})
	})
}
