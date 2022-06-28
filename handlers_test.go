package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestHandlers(t *testing.T) {
	spec.Run(t, "Handlers", testHandlers, spec.Report(report.Terminal{}))
}

func testHandlers(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle.test")
		h.AssertNil(t, err)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#ReadGroup", func() {
		it("returns a group", func() {
			h.Mkfile(t, `group = [{id = "A", version = "v1"}, {id = "B", extension = true, optional = true}]`,
				filepath.Join(tmpDir, "group.toml"),
			)
			actual, err := lifecycle.ReadGroup(filepath.Join(tmpDir, "group.toml"))
			h.AssertNil(t, err)
			if s := cmp.Diff(actual, buildpack.Group{
				Group: []buildpack.GroupElement{
					{ID: "A", Version: "v1"},
					{ID: "B", Extension: true, Optional: true},
				},
			}); s != "" {
				t.Fatalf("Unexpected list:\n%s\n", s)
			}
		})
	})

	when("#ReadOrder", func() {
		it("returns an ordering of buildpacks", func() {
			h.Mkfile(t,
				"[[order]]\n"+
					`group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`+"\n"+
					"[[order]]\n"+
					`group = [{id = "C"}]`+"\n",
				filepath.Join(tmpDir, "order.toml"),
			)
			actual, _, err := lifecycle.ReadOrder(filepath.Join(tmpDir, "order.toml"))
			h.AssertNil(t, err)
			if s := cmp.Diff(actual, buildpack.Order{
				{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}, {ID: "B", Optional: true}}},
				{Group: []buildpack.GroupElement{{ID: "C"}}},
			}); s != "" {
				t.Fatalf("Unexpected list:\n%s\n", s)
			}
		})

		when("there are extensions", func() {
			it("returns an ordering of buildpacks and an ordering of extensions", func() {
				h.Mkfile(t,
					"[[order]]\n"+
						`group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`+"\n"+
						"[[order]]\n"+
						`group = [{id = "C"}]`+"\n"+
						"[[order-extensions]]\n"+
						`group = [{id = "D"}]`+"\n",
					filepath.Join(tmpDir, "order.toml"),
				)
				foundOrder, foundOrderExt, err := lifecycle.ReadOrder(filepath.Join(tmpDir, "order.toml"))
				h.AssertNil(t, err)
				if s := cmp.Diff(foundOrder, buildpack.Order{
					{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}, {ID: "B", Optional: true}}},
					{Group: []buildpack.GroupElement{{ID: "C"}}},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
				if s := cmp.Diff(foundOrderExt, buildpack.Order{
					{Group: []buildpack.GroupElement{{ID: "D", Extension: true}}},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
			})
		})
	})

	when("DefaultConfigHandler", func() {
		var (
			configHandler  *lifecycle.DefaultConfigHandler
			mockController *gomock.Controller
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			configHandler = lifecycle.NewConfigHandler()
		})

		it.After(func() {
			mockController.Finish()
		})

		when(".ReadGroup", func() {
			it("returns a group", func() {
				h.Mkfile(t, `group = [{id = "A", version = "v1"}, {id = "B", extension = true, optional = true}]`,
					filepath.Join(tmpDir, "group.toml"),
				)

				actual, err := configHandler.ReadGroup(filepath.Join(tmpDir, "group.toml"))
				h.AssertNil(t, err)

				if s := cmp.Diff(actual, []buildpack.GroupElement{
					{ID: "A", Version: "v1"},
					{ID: "B", Extension: true, Optional: true},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
			})
		})

		when(".ReadOrder", func() {
			it("returns an ordering of buildpacks", func() {
				h.Mkfile(t,
					"[[order]]\n"+
						`group = [{id = "A", version = "v1"}, {id = "B", version = "v1", optional = true}]`+"\n"+
						"[[order]]\n"+
						`group = [{id = "C", version = "v1"}]`+"\n",
					filepath.Join(tmpDir, "order.toml"),
				)

				actual, _, err := configHandler.ReadOrder(filepath.Join(tmpDir, "order.toml"))
				h.AssertNil(t, err)

				if s := cmp.Diff(actual, buildpack.Order{
					{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}, {ID: "B", Version: "v1", Optional: true}}},
					{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
			})

			when("there are extensions", func() {
				it("returns an ordering of buildpacks and an ordering of extensions", func() {
					h.Mkfile(t,
						"[[order]]\n"+
							`group = [{id = "A", version = "v1"}, {id = "B", version = "v1", optional = true}]`+"\n"+
							"[[order]]\n"+
							`group = [{id = "C", version = "v1"}]`+"\n"+
							"[[order-extensions]]\n"+
							`group = [{id = "D", version = "v1"}]`+"\n",
						filepath.Join(tmpDir, "order.toml"),
					)

					foundOrder, foundOrderExt, err := configHandler.ReadOrder(filepath.Join(tmpDir, "order.toml"))
					h.AssertNil(t, err)

					if s := cmp.Diff(foundOrder, buildpack.Order{
						{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}, {ID: "B", Version: "v1", Optional: true}}},
						{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
					}); s != "" {
						t.Fatalf("Unexpected list:\n%s\n", s)
					}
					if s := cmp.Diff(foundOrderExt, buildpack.Order{
						{Group: []buildpack.GroupElement{{ID: "D", Version: "v1", Extension: true}}},
					}); s != "" {
						t.Fatalf("Unexpected list:\n%s\n", s)
					}
				})
			})
		})
	})
}
