package lifecycle_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
)

func TestDetector(t *testing.T) {
	spec.Run(t, "Detector", testDetector, spec.Report(report.Terminal{}))
}

func testDetector(t *testing.T, when spec.G, it spec.S) {
	var (
		config         *lifecycle.DetectConfig
		outLog, errLog *bytes.Buffer
		tmpDir         string
		appDir         string
		platformDir    string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		platformDir = filepath.Join(tmpDir, "platform")
		appDir = filepath.Join(tmpDir, "app")
		mkdir(t, appDir, filepath.Join(platformDir, "env"))

		buildpacksDir := filepath.Join("testdata", "by-id")

		outLog = &bytes.Buffer{}
		errLog = &bytes.Buffer{}
		config = &lifecycle.DetectConfig{
			AppDir:        appDir,
			PlatformDir:   platformDir,
			BuildpacksDir: buildpacksDir,
			Out:           log.New(io.MultiWriter(outLog, it.Out()), "", 0),
			Err:           log.New(io.MultiWriter(errLog, it.Out()), "", 0),
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	mkappfile := func(data string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			mkfile(t, data, filepath.Join(appDir, p))
		}
	}
	toappfile := func(data string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			tofile(t, data, filepath.Join(appDir, p))
		}
	}

	when("#Detect", func() {
		it("should expand order-containing buildpack IDs", func() {
			mkappfile("100", "detect-status")

			_, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{{ID: "E", Version: "v1"}}},
			}.Detect(config)
			if err != lifecycle.ErrFail {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff("\n"+outLog.String(), outputFailureG); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}

			if errLog.Len() > 0 {
				t.Fatalf("Unexpected error: %s\n", errLog)
			}
		})

		it("should select the first passing group", func() {
			mkappfile("100", "detect-status")
			mkappfile("0", "detect-status-A-v1", "detect-status-B-v1")

			group, _, err := lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{{ID: "E", Version: "v1"}}},
			}.Detect(config)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(group, lifecycle.BuildpackGroup{
				Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1"},
					{ID: "B", Version: "v1"},
				},
			}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if s := cmp.Diff("\n"+outLog.String(), outputPassAv1Bv1); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}

			if errLog.Len() > 0 {
				t.Fatalf("Unexpected error: %s\n", errLog)
			}
		})

		when("a build plan is employed", func() {
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
				if err != lifecycle.ErrFail {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := outLog.String(); !strings.HasSuffix(s,
					"skip: A@v1\n"+
						"pass: B@v1\n"+
						"pass: C@v1\n"+
						"fail: B@v1 requires dep1\n",
				) {
					t.Fatalf("Unexpected results:\n%s\n", s)
				}

				if errLog.Len() > 0 {
					t.Fatalf("Unexpected error: %s\n", errLog)
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
				if err != lifecycle.ErrFail {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := outLog.String(); !strings.HasSuffix(s,
					"pass: A@v1\n"+
						"pass: B@v1\n"+
						"skip: C@v1\n"+
						"fail: B@v1 provides unused dep1\n",
				) {
					t.Fatalf("Unexpected results:\n%s\n", s)
				}

				if errLog.Len() > 0 {
					t.Fatalf("Unexpected error: %s\n", errLog)
				}
			})
		})

		//it("should return the first matching group without any failed optional buildpacks", func() {
		//	mkfile(t, "1", filepath.Join(appDir, "add"))
		//	mkfile(t, "3", filepath.Join(appDir, "last"))
		//
		//	result := list[1]
		//	result.Group = result.Group[:len(result.Group)-1]
		//	plan, group := list.Detect(config)
		//	if s := cmp.Diff(*group, result); s != "" {
		//		t.Fatalf("Unexpected group:\n%s\n", s)
		//	}
		//	if s := cmp.Diff(string(plan), "[1]\n  1 = true\n\n[2]\n  2 = true\n\n[3]\n  3 = true\n"); s != "" {
		//		t.Fatalf("Unexpected plan:\n%s\n", s)
		//	}
		//
		//	if !strings.HasSuffix(outLog.String(),
		//		"======== Output: buildpack4-name ========\n"+
		//			"stdout: 4\nstderr: 4\n"+
		//			"======== Results ========\n"+
		//			"pass: buildpack1-name\npass: buildpack2-name\npass: buildpack3-name\nskip: buildpack4-name\n",
		//	) {
		//		t.Fatalf("Unexpected log: %s\n", outLog)
		//	}
		//
		//	if errLog.Len() > 0 {
		//		t.Fatalf("Unexpected error: %s\n", errLog)
		//	}
		//})
		//
		//it("should return empty if no groups match", func() {
		//	mkfile(t, "1", filepath.Join(appDir, "add"))
		//	mkfile(t, "0", filepath.Join(appDir, "last"))
		//
		//	if plan, group := list.Detect(config); group != nil {
		//		t.Fatalf("Unexpected group: %#v\n", group)
		//	} else if len(plan) > 0 {
		//		t.Fatalf("Unexpected plan: %s\n", string(plan))
		//	}
		//
		//	if !strings.HasSuffix(outLog.String(),
		//		"======== Output: buildpack2-name ========\n"+
		//			"stdout: 1\nstderr: 1\n"+
		//			"======== Results ========\n"+
		//			"fail: buildpack1-name\nfail: buildpack2-name\n",
		//	) {
		//		t.Fatalf("Unexpected log: %s\n", outLog)
		//	}
		//
		//	if errLog.Len() > 0 {
		//		t.Fatalf("Unexpected error: %s\n", errLog)
		//	}
		//})
		//
		//it("should return empty there is an error", func() {
		//	mkfile(t, "1", filepath.Join(appDir, "add"))
		//	mkfile(t, "error", filepath.Join(platformDir, "env", "ERROR"))
		//
		//	if plan, group := list.Detect(config); group != nil {
		//		t.Fatalf("Unexpected group: %#v\n", group)
		//	} else if len(plan) > 0 {
		//		t.Fatalf("Unexpected plan: %s\n", string(plan))
		//	}
		//
		//	if !strings.HasSuffix(outLog.String(),
		//		"======== Output: buildpack2-name ========\n"+
		//			"stdout: 1\nstderr: 1\n"+
		//			"======== Results ========\n"+
		//			"err:  buildpack1-name: (1)\nerr:  buildpack2-name: (1)\n",
		//	) {
		//		t.Fatalf("Unexpected log: %s\n", outLog)
		//	}
		//
		//	if errLog.Len() > 0 {
		//		t.Fatalf("Unexpected error: %s\n", errLog)
		//	}
		//})
	})
}

var outputFailureG = `
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

var outputPassAv1Bv1 = `
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
pass: A@v1
fail: C@v1
pass: B@v1
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: B@v2 ========
detect out: B@v2
detect err: B@v2
======== Results ========
pass: A@v1
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
pass: A@v1
fail: C@v2
fail: D@v2
pass: B@v1
======== Output: A@v1 ========
detect out: A@v1
detect err: A@v1
======== Output: B@v1 ========
detect out: B@v1
detect err: B@v1
======== Results ========
pass: A@v1
pass: B@v1
`
