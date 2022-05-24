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
	"github.com/buildpacks/lifecycle/testmock"
)

func TestHandlers(t *testing.T) {
	spec.Run(t, "Handlers", testHandlers, spec.Report(report.Terminal{}))
}

func testHandlers(t *testing.T, when spec.G, it spec.S) {
	when("#ReadGroup", func() {
		var tmpDir string

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "lifecycle.test")
			h.AssertNil(t, err)
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
		})

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
		var tmpDir string

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "lifecycle.test")
			h.AssertNil(t, err)
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
		})

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
						"[[order-ext]]\n"+
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
					{Group: []buildpack.GroupElement{{ID: "D"}}},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
			})
		})
	})

	when("DefaultConfigHandler", func() {
		var (
			configHandler  *lifecycle.DefaultConfigHandler
			apiVerifier    *testmock.MockAPIVerifier
			mockController *gomock.Controller
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			apiVerifier = testmock.NewMockAPIVerifier(mockController)
			configHandler = lifecycle.NewConfigHandler(apiVerifier)
		})

		it.After(func() {
			mockController.Finish()
		})

		when(".ReadGroup", func() {
			var tmpDir string

			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "lifecycle.test")
				h.AssertNil(t, err)
			})

			it.After(func() {
				os.RemoveAll(tmpDir)
			})

			it("returns a group", func() {
				t.Log("verifies buildpack apis")
				apiVerifier.EXPECT().VerifyBuildpackAPIForBuildpack("A@v1", "0.2")
				apiVerifier.EXPECT().VerifyBuildpackAPIForExtension("B@", "0.2")

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
			var (
				tmpDir   string
				dirStore *testmock.MockDirStore
			)

			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "lifecycle.test")
				h.AssertNil(t, err)
				dirStore = testmock.NewMockDirStore(mockController)
			})

			it.After(func() {
				os.RemoveAll(tmpDir)
			})

			it("returns an ordering of buildpacks", func() {
				t.Log("verifies buildpack apis")
				bpA1 := testmock.NewMockBuildModule(mockController)
				bpB1 := testmock.NewMockBuildModule(mockController)
				bpC1 := testmock.NewMockBuildModule(mockController)
				dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
				bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.2",
					Buildpack: buildpack.Info{ID: "A", Version: "v1"},
				})
				apiVerifier.EXPECT().VerifyBuildpackAPIForBuildpack("A@v1", "0.2")
				dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
				bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.2",
					Buildpack: buildpack.Info{ID: "B", Version: "v1"},
				})
				apiVerifier.EXPECT().VerifyBuildpackAPIForBuildpack("B@v1", "0.2")
				dirStore.EXPECT().LookupBp("C", "v1").Return(bpC1, nil)
				bpC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.2",
					Buildpack: buildpack.Info{ID: "C", Version: "v1"},
				})
				apiVerifier.EXPECT().VerifyBuildpackAPIForBuildpack("C@v1", "0.2")

				h.Mkfile(t,
					"[[order]]\n"+
						`group = [{id = "A", version = "v1"}, {id = "B", version = "v1", optional = true}]`+"\n"+
						"[[order]]\n"+
						`group = [{id = "C", version = "v1"}]`+"\n",
					filepath.Join(tmpDir, "order.toml"),
				)

				actual, _, err := configHandler.ReadOrder(filepath.Join(tmpDir, "order.toml"), dirStore)
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
					t.Log("verifies buildpack apis")
					bpA1 := testmock.NewMockBuildModule(mockController)
					bpB1 := testmock.NewMockBuildModule(mockController)
					bpC1 := testmock.NewMockBuildModule(mockController)
					extD1 := testmock.NewMockBuildModule(mockController)
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
					bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
					apiVerifier.EXPECT().VerifyBuildpackAPIForBuildpack("A@v1", "0.2")
					dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
					bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
					apiVerifier.EXPECT().VerifyBuildpackAPIForBuildpack("B@v1", "0.2")
					dirStore.EXPECT().LookupBp("C", "v1").Return(bpC1, nil)
					bpC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
					apiVerifier.EXPECT().VerifyBuildpackAPIForBuildpack("C@v1", "0.2")
					dirStore.EXPECT().LookupExt("D", "v1").Return(extD1, nil)
					extD1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
					apiVerifier.EXPECT().VerifyBuildpackAPIForExtension("D@v1", "0.2")

					h.Mkfile(t,
						"[[order]]\n"+
							`group = [{id = "A", version = "v1"}, {id = "B", version = "v1", optional = true}]`+"\n"+
							"[[order]]\n"+
							`group = [{id = "C", version = "v1"}]`+"\n"+
							"[[order-ext]]\n"+
							`group = [{id = "D", version = "v1"}]`+"\n",
						filepath.Join(tmpDir, "order.toml"),
					)

					foundOrder, foundOrderExt, err := configHandler.ReadOrder(filepath.Join(tmpDir, "order.toml"), dirStore)
					h.AssertNil(t, err)

					if s := cmp.Diff(foundOrder, buildpack.Order{
						{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}, {ID: "B", Version: "v1", Optional: true}}},
						{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
					}); s != "" {
						t.Fatalf("Unexpected list:\n%s\n", s)
					}
					if s := cmp.Diff(foundOrderExt, buildpack.Order{
						{Group: []buildpack.GroupElement{{ID: "D", Version: "v1"}}},
					}); s != "" {
						t.Fatalf("Unexpected list:\n%s\n", s)
					}
				})
			})
		})
	})
}
