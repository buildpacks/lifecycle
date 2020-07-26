package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestRootBuilder(t *testing.T) {
	spec.Run(t, "RootBuilder", testRootBuilder, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package testmock -destination testmock/env.go github.com/buildpacks/lifecycle RootBuildEnv

func testRootBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		builder        *lifecycle.RootBuilder
		mockCtrl       *gomock.Controller
		env            *testmock.MockBuildEnv
		stdout, stderr *bytes.Buffer
		tmpDir         string
		platformDir    string
		layersDir      string
		rootDir        string
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockBuildEnv(mockCtrl)

		var err error

		// Using the default tmp dir causes kaniko to go haywire for some reason
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		tmpDir, err = ioutil.TempDir(cwd, "tmp")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}

		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		rootDir = filepath.Join(tmpDir, "root")
		layersDir = filepath.Join(rootDir, "layers")
		platformDir = filepath.Join(rootDir, "platform")
		mkdir(t, rootDir, layersDir, filepath.Join(platformDir, "env"))

		outLog := log.New(io.MultiWriter(stdout, it.Out()), "", 0)
		errLog := log.New(io.MultiWriter(stderr, it.Out()), "", 0)

		buildpacksDir := filepath.Join("testdata", "by-id")

		builder = &lifecycle.RootBuilder{
			RootDir:       rootDir,
			PlatformDir:   platformDir,
			LayersDir:     layersDir,
			BuildpacksDir: buildpacksDir,
			Env:           env,
			Group: lifecycle.BuildpackGroup{
				Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1"},
					{ID: "B", Version: "v2"},
				},
			},
			Out: outLog,
			Err: errLog,
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Build", func() {
		when("building succeeds", func() {
			it.Before(func() {
				env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Bv2"), nil)
			})

			it("should ensure something", func() {
				if _, err := builder.Build(); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testExists(t,
					filepath.Join(layersDir, "A.tgz"),
					filepath.Join(layersDir, "B.tgz"),
				)

				data, err := os.Open(filepath.Join(layersDir, "A.tgz"))
				if err != nil {
					log.Fatal(err)
				}
				defer data.Close()

				tr := tar.NewReader(data)
				for {
					hdr, err := tr.Next()
					if err == io.EOF {
						break // End of archive
					}

					if err != nil {
						t.Fatalf("Error: %s\n", err)
					}
					fmt.Printf("Contents of %s:\n", hdr.Name)


					switch hdr.Name {
					case "/":
						continue
					case "build-env-A-v1/":
						continue
					case "build-env-cnb-buildpack-dir-A-v1":
						var b bytes.Buffer
						if _, err := io.Copy(&b, tr); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						if ! strings.HasSuffix(b.String(), "testdata/by-id/A/v1") {
							t.Fatalf("Error: %s\n", err)
						}
					case "build-info-A-v1":
						var b bytes.Buffer
						if _, err := io.Copy(&b, tr); err != nil {
							t.Fatalf("Unexpected info:\n%s\n", err)
						}

						if s := cmp.Diff(b.String(),
							"TEST_ENV: Av1\n",
						); s != "" {
							t.Fatalf("Unexpected info:\n%s\n", s)
						}
					case "build-plan-in-A-v1.toml":
						continue
					}
				}
			})

			it("should return build metadata when processes are present", func() {
				mkfile(t,
					`[[processes]]`+"\n"+
						`type = "A-type"`+"\n"+
						`command = "A-cmd"`+"\n"+
						`[[processes]]`+"\n"+
						`type = "override-type"`+"\n"+
						`command = "A-cmd"`+"\n",
					filepath.Join(rootDir, "launch-A-v1.toml"),
				)
				mkfile(t,
					`[[processes]]`+"\n"+
						`type = "B-type"`+"\n"+
						`command = "B-cmd"`+"\n"+
						`[[processes]]`+"\n"+
						`type = "override-type"`+"\n"+
						`command = "B-cmd"`+"\n",
					filepath.Join(rootDir, "launch-B-v2.toml"),
				)
				metadata, err := builder.Build()
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(metadata, &lifecycle.BuildMetadata{
					Processes: []launch.Process{
						{Type: "A-type", Command: "A-cmd"},
						{Type: "B-type", Command: "B-cmd"},
						{Type: "override-type", Command: "B-cmd"},
					},
					Buildpacks: []lifecycle.Buildpack{
						{ID: "A", Version: "v1"},
						{ID: "B", Version: "v2"},
					},
				}); s != "" {
					t.Fatalf("Unexpected metadata:\n%s\n", s)
				}
			})

			it("should return build metadata when processes are not present", func() {
				metadata, err := builder.Build()
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(metadata, &lifecycle.BuildMetadata{
					Processes: []launch.Process{},
					Buildpacks: []lifecycle.Buildpack{
						{ID: "A", Version: "v1"},
						{ID: "B", Version: "v2"},
					},
				}); s != "" {
					t.Fatalf("Unexpected:\n%s\n", s)
				}
			})

			it("should provide the platform dir", func() {
				mkfile(t, "some-data",
					filepath.Join(platformDir, "env", "SOME_VAR"),
				)
				if _, err := builder.Build(); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testExists(t,
					filepath.Join(rootDir, "build-env-A-v1", "SOME_VAR"),
					filepath.Join(rootDir, "build-env-B-v2", "SOME_VAR"),
				)
			})
		})
	})
}
