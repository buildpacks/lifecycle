package files_test

import (
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestStack(t *testing.T) {
	spec.Run(t, "Stack", testStack)
}

func testStack(t *testing.T, when spec.G, it spec.S) {
	when("stack.toml", func() {
		when("BestRunImageMirrorFor", func() {
			var stackMD *files.Stack

			it.Before(func() {
				stackMD = &files.Stack{RunImage: files.RunImageForExport{
					Image: "first.com/org/repo",
					Mirrors: []string{
						"myorg/myrepo",
						"zonal.gcr.io/org/repo",
						"gcr.io/org/repo",
					},
				}}
			})

			when("repoName is dockerhub", func() {
				it("returns the dockerhub image", func() {
					name, err := stackMD.BestRunImageMirrorFor("index.docker.io")
					h.AssertNil(t, err)
					h.AssertEq(t, name, "myorg/myrepo")
				})
			})

			when("registry is gcr.io", func() {
				it("returns the gcr.io image", func() {
					name, err := stackMD.BestRunImageMirrorFor("gcr.io")
					h.AssertNil(t, err)
					h.AssertEq(t, name, "gcr.io/org/repo")
				})

				when("registry is zonal.gcr.io", func() {
					it("returns the gcr image", func() {
						name, err := stackMD.BestRunImageMirrorFor("zonal.gcr.io")
						h.AssertNil(t, err)
						h.AssertEq(t, name, "zonal.gcr.io/org/repo")
					})
				})

				when("registry is missingzone.gcr.io", func() {
					it("returns the run image", func() {
						name, err := stackMD.BestRunImageMirrorFor("missingzone.gcr.io")
						h.AssertNil(t, err)
						h.AssertEq(t, name, "first.com/org/repo")
					})
				})
			})

			when("one of the images is non-parsable", func() {
				it.Before(func() {
					stackMD.RunImage.Mirrors = []string{"as@ohd@as@op", "gcr.io/myorg/myrepo"}
				})

				it("skips over it", func() {
					name, err := stackMD.BestRunImageMirrorFor("gcr.io")
					h.AssertNil(t, err)
					h.AssertEq(t, name, "gcr.io/myorg/myrepo")
				})
			})
		})
	})
}
