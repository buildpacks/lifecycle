package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDetector(t *testing.T) {
	spec.Run(t, "Detector", testDetector, spec.Report(report.Terminal{}))
}

func testDetector(t *testing.T, when spec.G, it spec.S) {
	var (
		config      *lifecycle.DetectConfig
		platformDir string
		tmpDir      string
		logHandler  *memory.Handler
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		platformDir = filepath.Join(tmpDir, "platform")
		appDir := filepath.Join(tmpDir, "app")
		mkdir(t, appDir, filepath.Join(platformDir, "env"))

		buildpacksDir := filepath.Join("testdata", "by-id")

		logHandler = memory.New()
		config = &lifecycle.DetectConfig{
			FullEnv:       append(os.Environ(), "ENV_TYPE=full"),
			ClearEnv:      append(os.Environ(), "ENV_TYPE=clear"),
			AppDir:        appDir,
			PlatformDir:   platformDir,
			BuildpacksDir: buildpacksDir,
			Logger:        &log.Logger{Handler: logHandler},
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	mkappfile := func(data string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			mkfile(t, data, filepath.Join(config.AppDir, p))
		}
	}
	toappfile := func(data string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			tofile(t, data, filepath.Join(config.AppDir, p))
		}
	}
	rdappfile := func(path string) string {
		t.Helper()
		return rdfile(t, filepath.Join(config.AppDir, path))
	}

	when("#Detect", func() {
		it("should expand order-containing buildpack IDs", func() {
			mkappfile("100", "detect-status")

			_, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{{ID: "E", Version: "v1"}}},
			}.Detect(config)
			if err, ok := err.(*lifecycle.Error); !ok || err.Type != lifecycle.ErrTypeFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff("\n"+allLogs(logHandler), outputFailureEv1); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should select the first passing group", func() {
			mkappfile("100", "detect-status")
			mkappfile("0", "detect-status-A-v1", "detect-status-B-v1")

			group, plan, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{{ID: "E", Version: "v1"}}},
			}.Detect(config)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(group, lifecycle.BuildpackGroup{
				Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1", API: "0.3"},
					{ID: "B", Version: "v1", API: "0.2"},
				},
			}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(plan.Entries, []lifecycle.BuildPlanEntry(nil)) {
				t.Fatalf("Unexpected entries:\n%+v\n", plan.Entries)
			}

			if s := allLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"pass: A@v1\n"+
					"pass: B@v1\n"+
					"Resolving plan... (try #1)\n"+
					"A v1\n"+
					"B v1\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fail if the group is empty", func() {
			_, _, err := lifecycle.BuildpackOrder([]lifecycle.BuildpackGroup{{}}).Detect(config)
			if err, ok := err.(*lifecycle.Error); !ok || err.Type != lifecycle.ErrTypeFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(allLogs(logHandler),
				"======== Results ========\n"+
					"Resolving plan... (try #1)\n"+
					"fail: no viable buildpacks in group\n",
			); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fail if the group has no viable buildpacks, even if no required buildpacks fail", func() {
			mkappfile("100", "detect-status")
			_, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1", Optional: true},
					{ID: "B", Version: "v1", Optional: true},
				}},
			}.Detect(config)
			if err, ok := err.(*lifecycle.Error); !ok || err.Type != lifecycle.ErrTypeFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := allLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"skip: A@v1\n"+
					"skip: B@v1\n"+
					"Resolving plan... (try #1)\n"+
					"fail: no viable buildpacks in group\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fail with specific error if any bp detect fails in an unexpected way", func() {
			mkappfile("100", "detect-status")
			mkappfile("0", "detect-status-A-v1")
			mkappfile("127", "detect-status-B-v1")
			_, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1", Optional: false},
					{ID: "B", Version: "v1", Optional: false},
				}},
			}.Detect(config)
			if err, ok := err.(*lifecycle.Error); !ok || err.Type != lifecycle.ErrTypeBuildpack {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := allLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"pass: A@v1\n"+
					"err:  B@v1 (127)\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should select an appropriate env type", func() {
			mkappfile("0", "detect-status-A-v1.clear", "detect-status-B-v1")

			_, _, err := lifecycle.BuildpackOrder{{
				Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1.clear"},
					{ID: "B", Version: "v1"},
				},
			}}.Detect(config)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if typ := rdappfile("detect-env-type-A-v1.clear"); typ != "clear" {
				t.Fatalf("Unexpected env type: %s\n", typ)
			}

			if typ := rdappfile("detect-env-type-B-v1"); typ != "full" {
				t.Fatalf("Unexpected env type: %s\n", typ)
			}
		})

		it("should set CNB_BUILDPACK_DIR in the environment", func() {
			mkappfile("0", "detect-status-A-v1.clear", "detect-status-B-v1")

			_, _, err := lifecycle.BuildpackOrder{{
				Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1.clear"},
					{ID: "B", Version: "v2"},
				},
			}}.Detect(config)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			bpsDir, err := filepath.Abs(config.BuildpacksDir)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}
			expectedBpDir := filepath.Join(bpsDir, "A/v1.clear")
			if bpDir := rdappfile("detect-env-cnb-buildpack-dir-A-v1.clear"); bpDir != expectedBpDir {
				t.Fatalf("Unexpected buildpack dir:\n\twanted: %s\n\tgot: %s\n", expectedBpDir, bpDir)
			}

			expectedBpDir = filepath.Join(bpsDir, "B/v2")
			if bpDir := rdappfile("detect-env-cnb-buildpack-dir-B-v2"); bpDir != expectedBpDir {
				t.Fatalf("Unexpected buildpack dir:\n\twanted: %s\n\tgot: %s\n", expectedBpDir, bpDir)
			}
		})

		it("should not output detect pass and fail as info level", func() {
			mkappfile("100", "detect-status")
			mkappfile("0", "detect-status-A-v1")
			mkappfile("100", "detect-status-B-v1")
			config.Logger = &log.Logger{Handler: logHandler, Level: log.InfoLevel}

			_, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1", Optional: false},
					{ID: "B", Version: "v1", Optional: false},
				}},
			}.Detect(config)
			if err, ok := err.(*lifecycle.Error); !ok || err.Type != lifecycle.ErrTypeFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := allLogs(logHandler); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should output detect errors as info level", func() {
			mkappfile("100", "detect-status")
			mkappfile("0", "detect-status-A-v1")
			mkappfile("127", "detect-status-B-v1")
			config.Logger = &log.Logger{Handler: logHandler, Level: log.InfoLevel}

			_, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1", Optional: false},
					{ID: "B", Version: "v1", Optional: false},
				}},
			}.Detect(config)
			if err, ok := err.(*lifecycle.Error); !ok || err.Type != lifecycle.ErrTypeBuildpack {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := allLogs(logHandler); !strings.HasSuffix(s,
				"======== Output: B@v1 ========\n"+
					"detect out: B@v1\n"+
					"detect err: B@v1\n"+
					"err:  B@v1 (127)\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		when("a build plan is employed", func() {
			it("should return a build plan with matched dependencies", func() {
				mkappfile("100", "detect-status-C-v1")
				mkappfile("100", "detect-status-B-v2")

				toappfile("\n[[provides]]\n name = \"dep1\"", "detect-plan-A-v1.toml", "detect-plan-C-v2.toml")
				toappfile("\n[[provides]]\n name = \"dep2\"", "detect-plan-A-v1.toml", "detect-plan-C-v2.toml")
				toappfile("\n[[provides]]\n name = \"dep2\"", "detect-plan-D-v2.toml")

				toappfile("\n[[requires]]\n name = \"dep1\"", "detect-plan-D-v2.toml", "detect-plan-B-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep2\"", "detect-plan-D-v2.toml", "detect-plan-B-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep2\"", "detect-plan-A-v1.toml")

				group, plan, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1"},
						{ID: "C", Version: "v2"},
						{ID: "D", Version: "v2"},
						{ID: "B", Version: "v1"},
					}},
				}.Detect(config)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := cmp.Diff(group, lifecycle.BuildpackGroup{
					Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1", API: "0.3"},
						{ID: "C", Version: "v2", API: "0.2"},
						{ID: "D", Version: "v2", API: "0.2"},
						{ID: "B", Version: "v1", API: "0.2"},
					},
				}); s != "" {
					t.Fatalf("Unexpected group:\n%s\n", s)
				}

				if !hasEntries(plan.Entries, []lifecycle.BuildPlanEntry{
					{
						Providers: []lifecycle.Buildpack{
							{ID: "A", Version: "v1"},
							{ID: "C", Version: "v2"},
						},
						Requires: []lifecycle.Require{{Name: "dep1"}, {Name: "dep1"}},
					},
					{
						Providers: []lifecycle.Buildpack{
							{ID: "A", Version: "v1"},
							{ID: "C", Version: "v2"},
							{ID: "D", Version: "v2"},
						},
						Requires: []lifecycle.Require{{Name: "dep2"}, {Name: "dep2"}, {Name: "dep2"}},
					},
				}) {
					t.Fatalf("Unexpected entries:\n%+v\n", plan.Entries)
				}

				if s := allLogs(logHandler); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"pass: A@v1\n"+
						"pass: C@v2\n"+
						"pass: D@v2\n"+
						"pass: B@v1\n"+
						"Resolving plan... (try #1)\n"+
						"A v1\n"+
						"C v2\n"+
						"D v2\n"+
						"B v1\n",
				) {
					t.Fatalf("Unexpected log:\n%s\n", s)
				}
			})

			it("should fail if all requires are not provided first", func() {
				toappfile("\n[[provides]]\n name = \"dep1\"", "detect-plan-A-v1.toml", "detect-plan-C-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep1\"", "detect-plan-B-v1.toml", "detect-plan-C-v1.toml")
				mkappfile("100", "detect-status-A-v1")

				_, _, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1", Optional: true},
						{ID: "B", Version: "v1"},
						{ID: "C", Version: "v1"},
					}},
				}.Detect(config)
				if err, ok := err.(*lifecycle.Error); !ok || err.Type != lifecycle.ErrTypeFailedDetection {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := allLogs(logHandler); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"skip: A@v1\n"+
						"pass: B@v1\n"+
						"pass: C@v1\n"+
						"Resolving plan... (try #1)\n"+
						"fail: B@v1 requires dep1\n",
				) {
					t.Fatalf("Unexpected log:\n%s\n", s)
				}
			})

			it("should fail if all provides are not required after", func() {
				toappfile("\n[[provides]]\n name = \"dep1\"", "detect-plan-A-v1.toml", "detect-plan-B-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep1\"", "detect-plan-A-v1.toml", "detect-plan-C-v1.toml")
				mkappfile("100", "detect-status-C-v1")

				_, _, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1"},
						{ID: "B", Version: "v1"},
						{ID: "C", Version: "v1", Optional: true},
					}},
				}.Detect(config)
				if err, ok := err.(*lifecycle.Error); !ok || err.Type != lifecycle.ErrTypeFailedDetection {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := allLogs(logHandler); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"pass: A@v1\n"+
						"pass: B@v1\n"+
						"skip: C@v1\n"+
						"Resolving plan... (try #1)\n"+
						"fail: B@v1 provides unused dep1\n",
				) {
					t.Fatalf("Unexpected log:\n%s\n", s)
				}
			})

			it("should succeed if unmet provides/requires are optional", func() {
				toappfile("\n[[requires]]\n name = \"dep-missing\"", "detect-plan-A-v1.toml")
				toappfile("\n[[provides]]\n name = \"dep-missing\"", "detect-plan-C-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep-present\"", "detect-plan-B-v1.toml")
				toappfile("\n[[provides]]\n name = \"dep-present\"", "detect-plan-B-v1.toml")

				group, plan, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1", Optional: true},
						{ID: "B", Version: "v1"},
						{ID: "C", Version: "v1", Optional: true},
					}},
				}.Detect(config)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := cmp.Diff(group, lifecycle.BuildpackGroup{
					Group: []lifecycle.Buildpack{
						{ID: "B", Version: "v1", API: "0.2"},
					},
				}); s != "" {
					t.Fatalf("Unexpected group:\n%s\n", s)
				}

				if !hasEntries(plan.Entries, []lifecycle.BuildPlanEntry{
					{
						Providers: []lifecycle.Buildpack{{ID: "B", Version: "v1"}},
						Requires:  []lifecycle.Require{{Name: "dep-present"}},
					},
				}) {
					t.Fatalf("Unexpected entries:\n%+v\n", plan.Entries)
				}

				if s := allLogs(logHandler); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"pass: A@v1\n"+
						"pass: B@v1\n"+
						"pass: C@v1\n"+
						"Resolving plan... (try #1)\n"+
						"skip: A@v1 requires dep-missing\n"+
						"skip: C@v1 provides unused dep-missing\n"+
						"1 of 3 buildpacks participating\n"+
						"B v1\n",
				) {
					t.Fatalf("Unexpected log:\n%s\n", s)
				}
			})

			it("should fallback to alternate build plans", func() {
				toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
				toappfile("\n[[or]]", "detect-plan-A-v1.toml")
				toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")

				toappfile("\n[[requires]]\n name = \"dep3-missing\"\n version=\"some-version\"", "detect-plan-B-v1.toml")
				toappfile("\n[requires.metadata]\n version=\"some-version\"", "detect-plan-B-v1.toml")
				toappfile("\n[[or]]", "detect-plan-B-v1.toml")
				toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version=\"some-version\"", "detect-plan-B-v1.toml")
				toappfile("\n[or.requires.metadata]\n version=\"some-version\"", "detect-plan-B-v1.toml")

				toappfile("\n[[requires]]\n name = \"dep4-missing\"", "detect-plan-C-v1.toml")
				toappfile("\n[[provides]]\n name = \"dep5-missing\"", "detect-plan-C-v1.toml")
				toappfile("\n[[or]]", "detect-plan-C-v1.toml")
				toappfile("\n[[or.requires]]\n name = \"dep6-present\"", "detect-plan-C-v1.toml")
				toappfile("\n[[or.provides]]\n name = \"dep6-present\"", "detect-plan-C-v1.toml")

				toappfile("\n[[requires]]\n name = \"dep7-missing\"", "detect-plan-D-v1.toml")
				toappfile("\n[[provides]]\n name = \"dep8-missing\"", "detect-plan-D-v1.toml")
				toappfile("\n[[or]]", "detect-plan-D-v1.toml")
				toappfile("\n[[or.requires]]\n name = \"dep9-missing\"", "detect-plan-D-v1.toml")
				toappfile("\n[[or.provides]]\n name = \"dep10-missing\"", "detect-plan-D-v1.toml")

				group, plan, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1", Optional: true},
						{ID: "B", Version: "v1", Optional: true},
						{ID: "C", Version: "v1"},
						{ID: "D", Version: "v1", Optional: true},
					}},
				}.Detect(config)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := cmp.Diff(group, lifecycle.BuildpackGroup{
					Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1", API: "0.3"},
						{ID: "B", Version: "v1", API: "0.2"},
						{ID: "C", Version: "v1", API: "0.2"},
					},
				}); s != "" {
					t.Fatalf("Unexpected group:\n%s\n", s)
				}

				if !hasEntries(plan.Entries, []lifecycle.BuildPlanEntry{
					{
						Providers: []lifecycle.Buildpack{{ID: "A", Version: "v1"}},
						Requires:  []lifecycle.Require{{Name: "dep1-present", Metadata: map[string]interface{}{"version": "some-version"}}},
					},
					{
						Providers: []lifecycle.Buildpack{{ID: "C", Version: "v1"}},
						Requires:  []lifecycle.Require{{Name: "dep6-present"}},
					},
				}) {
					t.Fatalf("Unexpected entries:\n%+v\n", plan.Entries)
				}

				if s := allLogs(logHandler); !strings.HasSuffix(s,
					"Resolving plan... (try #16)\n"+
						"skip: D@v1 requires dep9-missing\n"+
						"skip: D@v1 provides unused dep10-missing\n"+
						"3 of 4 buildpacks participating\n"+
						"A v1\n"+
						"B v1\n"+
						"C v1\n",
				) {
					t.Fatalf("Unexpected log:\n%s\n", s)
				}
			})

			it("should convert top level versions to metadata versions", func() {
				mkappfile("100", "detect-status-C-v1")
				mkappfile("100", "detect-status-B-v2")

				toappfile("\n[[provides]]\n name = \"dep1\"\n version = \"some-version\"", "detect-plan-A-v1.toml", "detect-plan-C-v2.toml")
				toappfile("\n[[provides]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml", "detect-plan-C-v2.toml")
				toappfile("\n[[provides]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-D-v2.toml")

				toappfile("\n[[requires]]\n name = \"dep1\"\n version = \"some-version\"", "detect-plan-D-v2.toml", "detect-plan-B-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-D-v2.toml", "detect-plan-B-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

				group, plan, err := lifecycle.BuildpackOrder{
					{Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1"},
						{ID: "C", Version: "v2"},
						{ID: "D", Version: "v2"},
						{ID: "B", Version: "v1"},
					}},
				}.Detect(config)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := cmp.Diff(group, lifecycle.BuildpackGroup{
					Group: []lifecycle.Buildpack{
						{ID: "A", Version: "v1", API: "0.3"},
						{ID: "C", Version: "v2", API: "0.2"},
						{ID: "D", Version: "v2", API: "0.2"},
						{ID: "B", Version: "v1", API: "0.2"},
					},
				}); s != "" {
					t.Fatalf("Unexpected group:\n%s\n", s)
				}

				if !hasEntries(plan.Entries, []lifecycle.BuildPlanEntry{
					{
						Providers: []lifecycle.Buildpack{
							{ID: "A", Version: "v1"},
							{ID: "C", Version: "v2"},
						},
						Requires: []lifecycle.Require{
							{Name: "dep1", Metadata: map[string]interface{}{"version": "some-version"}},
							{Name: "dep1", Metadata: map[string]interface{}{"version": "some-version"}},
						},
					},
					{
						Providers: []lifecycle.Buildpack{
							{ID: "A", Version: "v1"},
							{ID: "C", Version: "v2"},
							{ID: "D", Version: "v2"},
						},
						Requires: []lifecycle.Require{
							{Name: "dep2", Metadata: map[string]interface{}{"version": "some-version"}},
							{Name: "dep2", Metadata: map[string]interface{}{"version": "some-version"}},
							{Name: "dep2", Metadata: map[string]interface{}{"version": "some-version"}},
						},
					},
				}) {
					t.Fatalf("Unexpected entries:\n%+v\n", plan.Entries)
				}
			})

			when("BuildpackTOML.Detect()", func() {
				it("should fail if buildpacks with buildpack api 0.2 have a top level version and a metadata version that are different", func() {
					bpPath, err := filepath.Abs(filepath.Join("testdata", "by-id", "D", "v2"))
					h.AssertNil(t, err)
					bpTOML := lifecycle.BuildpackTOML{
						API: "0.2",
						Buildpack: lifecycle.BuildpackInfo{
							ID:      "D",
							Version: "v2",
							Name:    "Buildpack D",
						},
						Path: bpPath,
					}
					toappfile("\n[[provides]]\n name = \"dep2\"", "detect-plan-D-v2.toml")
					toappfile("\n[[requires]]\n name = \"dep1\"\n version = \"some-version\"", "detect-plan-D-v2.toml")
					toappfile("\n[requires.metadata]\n version = \"some-other-version\"", "detect-plan-D-v2.toml")

					detectRun := bpTOML.Detect(config)

					h.AssertEq(t, detectRun.Code, -1)
					err = detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), `buildpack D has a "version" key that does not match "metadata.version"`)
				})

				it("should fail if buildpack with buildpack api 0.2 has alternate build plan with a top level version and a metadata version that are different", func() {
					bpPath, err := filepath.Abs(filepath.Join("testdata", "by-id", "B", "v1"))
					h.AssertNil(t, err)
					bpTOML := lifecycle.BuildpackTOML{
						API: "0.2",
						Buildpack: lifecycle.BuildpackInfo{
							ID:      "B",
							Version: "v1",
							Name:    "Buildpack B",
						},
						Path: bpPath,
					}
					toappfile("\n[[requires]]\n name = \"dep3-missing\"", "detect-plan-B-v1.toml")
					toappfile("\n[[or]]", "detect-plan-B-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-B-v1.toml")
					toappfile("\n[or.requires.metadata]\n version = \"some-other-version\"", "detect-plan-B-v1.toml")

					detectRun := bpTOML.Detect(config)

					h.AssertEq(t, detectRun.Code, -1)
					err = detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}

					h.AssertEq(t, err.Error(), `buildpack B has a "version" key that does not match "metadata.version"`)
				})

				it("should fail if buildpacks with buildpack api 0.3+ have both a top level version and a metadata version", func() {
					bpPath, err := filepath.Abs(filepath.Join("testdata", "by-id", "A", "v1"))
					h.AssertNil(t, err)
					bpTOML := lifecycle.BuildpackTOML{
						API: "0.3",
						Buildpack: lifecycle.BuildpackInfo{
							ID:      "A",
							Version: "v1",
							Name:    "Buildpack A",
						},
						Path: bpPath,
					}
					toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[requires.metadata]\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := bpTOML.Detect(config)

					h.AssertEq(t, detectRun.Code, -1)
					err = detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}

					h.AssertEq(t, err.Error(), `buildpack A has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`)
				})

				it("should fail if buildpack with buildpack api 0.3+ has alternate build plan with both a top level version and a metadata version", func() {
					bpPath, err := filepath.Abs(filepath.Join("testdata", "by-id", "A", "v1"))
					h.AssertNil(t, err)
					bpTOML := lifecycle.BuildpackTOML{
						API: "0.3",
						Buildpack: lifecycle.BuildpackInfo{
							ID:      "A",
							Version: "v1",
							Name:    "Buildpack A",
						},
						Path: bpPath,
					}
					toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[or.requires.metadata]\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := bpTOML.Detect(config)

					h.AssertEq(t, detectRun.Code, -1)
					err = detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}

					h.AssertEq(t, err.Error(), `buildpack A has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`)
				})

				it("should warn if buildpacks with buildpack api 0.3+ have a top level version", func() {
					bpPath, err := filepath.Abs(filepath.Join("testdata", "by-id", "A", "v1"))
					h.AssertNil(t, err)
					bpTOML := lifecycle.BuildpackTOML{
						API: "0.3",
						Buildpack: lifecycle.BuildpackInfo{
							ID:      "A",
							Version: "v1",
							Name:    "Buildpack A",
						},
						Path: bpPath,
					}
					toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := bpTOML.Detect(config)

					h.AssertEq(t, detectRun.Code, 0)
					err = detectRun.Err
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := allLogs(logHandler); !strings.Contains(s,
						`Warning: buildpack A has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`,
					) {
						t.Fatalf("Expected log to contain warning:\n%s\n", s)
					}
				})

				it("should warn if buildpack with buildpack api 0.3+ has alternate build plan with a top level version", func() {
					bpPath, err := filepath.Abs(filepath.Join("testdata", "by-id", "A", "v1"))
					h.AssertNil(t, err)
					bpTOML := lifecycle.BuildpackTOML{
						API: "0.3",
						Buildpack: lifecycle.BuildpackInfo{
							ID:      "A",
							Version: "v1",
							Name:    "Buildpack A",
						},
						Path: bpPath,
					}
					toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := bpTOML.Detect(config)

					h.AssertEq(t, detectRun.Code, 0)
					err = detectRun.Err
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := allLogs(logHandler); !strings.Contains(s,
						`Warning: buildpack A has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`,
					) {
						t.Fatalf("Expected log to contain warning:\n%s\n", s)
					}
				})
			})
		})
	})
}

func hasEntry(l []lifecycle.BuildPlanEntry, entry lifecycle.BuildPlanEntry) bool {
	for _, e := range l {
		if reflect.DeepEqual(e, entry) {
			return true
		}
	}
	return false
}

func hasEntries(a, b []lifecycle.BuildPlanEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for _, e := range a {
		if !hasEntry(b, e) {
			return false
		}
	}
	return true
}

func allLogs(logHandler *memory.Handler) string {
	var out string
	for _, le := range logHandler.Entries {
		out = out + le.Message + "\n"
	}
	return cleanEndings(out)
}

const outputFailureEv1 = `
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: C@v1 ========
detect out: C@v1
detect err: C@v1
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
fail: A@v1
fail: C@v1
fail: B@v1
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: B@v2 ========
detect out: B@v2
detect err: B@v2
======== Results ========
fail: A@v1
fail: B@v2
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: C@v2 ========
detect out: C@v2
detect err: C@v2
======== Output: D@v2 ========
detect out: D@v2
detect err: D@v2
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
fail: A@v1
fail: C@v2
fail: D@v2
fail: B@v1
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
fail: A@v1
fail: B@v1
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: D@v1 ========
detect out: D@v1
detect err: D@v1
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
fail: A@v1
fail: D@v1
fail: B@v1
`
