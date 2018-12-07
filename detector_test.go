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
		list           lifecycle.BuildpackOrder
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

		outLog = &bytes.Buffer{}
		errLog = &bytes.Buffer{}
		config = &lifecycle.DetectConfig{
			AppDir:      appDir,
			PlatformDir: platformDir,
			Out:         log.New(io.MultiWriter(outLog, it.Out()), "", 0),
			Err:         log.New(io.MultiWriter(errLog, it.Out()), "", 0),
		}

		buildpackDir := filepath.Join("testdata", "buildpack")
		list = lifecycle.BuildpackOrder{
			{
				Buildpacks: []*lifecycle.Buildpack{
					{Name: "buildpack1-name", Dir: buildpackDir},
					{Name: "buildpack2-name", Dir: buildpackDir},
					{Name: "buildpack3-name", Dir: buildpackDir},
					{Name: "buildpack4-name", Dir: buildpackDir},
				},
			},
			{
				Buildpacks: []*lifecycle.Buildpack{
					{Name: "buildpack1-name", Dir: buildpackDir},
					{Name: "buildpack2-name", Dir: buildpackDir},
					{Name: "buildpack3-name", Dir: buildpackDir},
					{Name: "buildpack4-name", Dir: buildpackDir, Optional: true},
				},
			},
			{
				Buildpacks: []*lifecycle.Buildpack{
					{Name: "buildpack1-name", Dir: buildpackDir, Optional: true},
				},
			},
			{
				Buildpacks: []*lifecycle.Buildpack{
					{Name: "buildpack1-name", Dir: buildpackDir},
					{Name: "buildpack2-name", Dir: buildpackDir},
					{Name: "buildpack3-name", Dir: buildpackDir},
				},
			},
			{
				Buildpacks: []*lifecycle.Buildpack{
					{Name: "buildpack1-name", Dir: buildpackDir},
					{Name: "buildpack2-name", Dir: buildpackDir},
				},
			},
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#Detect", func() {
		it("should return the first matching group without optional buildpacks", func() {
			mkfile(t, "1", filepath.Join(appDir, "add"))
			mkfile(t, "3", filepath.Join(appDir, "last"))

			result := list[1]
			result.Buildpacks = result.Buildpacks[:len(result.Buildpacks)-1]
			plan, group := list.Detect(config)
			if s := cmp.Diff(*group, result); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}
			if s := cmp.Diff(string(plan), "[1]\n  1 = true\n\n[2]\n  2 = true\n\n[3]\n  3 = true\n"); s != "" {
				t.Fatalf("Unexpected plan:\n%s\n", s)
			}

			if !strings.HasSuffix(outLog.String(),
				"======== Output: buildpack4-name ========\n"+
					"stdout: 4\nstderr: 4\n"+
					"======== Results ========\n"+
					"buildpack1-name: pass\nbuildpack2-name: pass\nbuildpack3-name: pass\nbuildpack4-name: skip\n",
			) {
				t.Fatalf("Unexpected log: %s\n", outLog)
			}

			if errLog.Len() > 0 {
				t.Fatalf("Unexpected error: %s\n", errLog)
			}
		})

		it("should return empty if no groups match", func() {
			mkfile(t, "1", filepath.Join(appDir, "add"))
			mkfile(t, "0", filepath.Join(appDir, "last"))

			if plan, group := list.Detect(config); group != nil {
				t.Fatalf("Unexpected group: %#v\n", group)
			} else if len(plan) > 0 {
				t.Fatalf("Unexpected plan: %s\n", string(plan))
			}

			if !strings.HasSuffix(outLog.String(),
				"======== Output: buildpack2-name ========\n"+
					"stdout: 1\nstderr: 1\n"+
					"======== Results ========\n"+
					"buildpack1-name: fail\nbuildpack2-name: fail\n",
			) {
				t.Fatalf("Unexpected log: %s\n", outLog)
			}

			if errLog.Len() > 0 {
				t.Fatalf("Unexpected error: %s\n", errLog)
			}
		})

		it("should return empty there is an error", func() {
			mkfile(t, "1", filepath.Join(appDir, "add"))
			mkfile(t, "error", filepath.Join(platformDir, "env", "ERROR"))

			if plan, group := list.Detect(config); group != nil {
				t.Fatalf("Unexpected group: %#v\n", group)
			} else if len(plan) > 0 {
				t.Fatalf("Unexpected plan: %s\n", string(plan))
			}

			if !strings.HasSuffix(outLog.String(),
				"======== Output: buildpack2-name ========\n"+
					"stdout: 1\nstderr: 1\n"+
					"======== Results ========\n"+
					"buildpack1-name: error (1)\nbuildpack2-name: error (1)\n",
			) {
				t.Fatalf("Unexpected log: %s\n", outLog)
			}

			if errLog.Len() > 0 {
				t.Fatalf("Unexpected error: %s\n", errLog)
			}
		})
	})
}
