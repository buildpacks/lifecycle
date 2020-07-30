package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/snapshot"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestBuilder(t *testing.T) {
	spec.Run(t, "Builder", testBuilder, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package testmock -destination testmock/env.go github.com/buildpacks/lifecycle BuildEnv

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		builder        *lifecycle.Builder
		mockCtrl       *gomock.Controller
		env            *testmock.MockBuildEnv
		stdout, stderr *bytes.Buffer
		outLog, errLog *log.Logger
		tmpDir         string
		platformDir    string
		appDir         string
		layersDir      string
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockBuildEnv(mockCtrl)

		// Using the default tmp dir causes kaniko to go haywire for some reason
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		tmpDir, err = ioutil.TempDir(filepath.Join(cwd, "tmp"), "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}

		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}

		outLog = log.New(io.MultiWriter(stdout, it.Out()), "", 0)
		errLog = log.New(io.MultiWriter(stderr, it.Out()), "", 0)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Build", func() {
		// we can't create the objects we need in the for loop, because it is evaluated before the it.Before, which
		// sets the vars we need. There are also objects that might return an err. I tried and it was a god-awful mess
		for _, builderType := range []string{"kaniko", "no-op"} {
			builderType := builderType

			it.Before(func() {
				if builderType == "kaniko" {
					builder = createRootBuilder(t, tmpDir, env, outLog, errLog)
				} else {
					builder = createBuilder(t, tmpDir, env, outLog, errLog)
				}
				appDir = builder.AppDir
				platformDir = builder.PlatformDir
				layersDir = builder.LayersDir
			})

			when("building succeeds", func() {
				it.Before(func() {
					env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
					env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Bv2"), nil)
				})

				it("should ensure each buildpack's layers dir exists and process build layers", func() {
					mkdir(t,
						filepath.Join(layersDir, "A"),

						filepath.Join(appDir, "layers-A-v1", "layer1"),
						filepath.Join(appDir, "layers-A-v1", "layer2"),
						filepath.Join(appDir, "layers-A-v1", "layer3"),
						filepath.Join(appDir, "layers-B-v2", "layer4"),
						filepath.Join(appDir, "layers-B-v2", "layer5"),
						filepath.Join(appDir, "layers-B-v2", "layer6"),
					)
					mkfile(t, "build = true",
						filepath.Join(appDir, "layers-A-v1", "layer1.toml"),
						filepath.Join(appDir, "layers-A-v1", "layer3.toml"),
						filepath.Join(appDir, "layers-B-v2", "layer4.toml"),
						filepath.Join(appDir, "layers-B-v2", "layer6.toml"),
					)
					gomock.InOrder(
						env.EXPECT().AddRootDir(filepath.Join(layersDir, "A", "layer1")),
						env.EXPECT().AddRootDir(filepath.Join(layersDir, "A", "layer3")),
						env.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer1", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer1", "env.build")),
						env.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer3", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer3", "env.build")),

						env.EXPECT().AddRootDir(filepath.Join(layersDir, "B", "layer4")),
						env.EXPECT().AddRootDir(filepath.Join(layersDir, "B", "layer6")),
						env.EXPECT().AddEnvDir(filepath.Join(layersDir, "B", "layer4", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(layersDir, "B", "layer4", "env.build")),
						env.EXPECT().AddEnvDir(filepath.Join(layersDir, "B", "layer6", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(layersDir, "B", "layer6", "env.build")),
					)
					if _, err := builder.Build(); err != nil {
						t.Fatalf("Error: %s\n", err)
					}
					testExists(t,
						filepath.Join(layersDir, "A"),
						filepath.Join(layersDir, "B"),
					)
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "A.tgz"))
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "B.tgz"))
				})

				it("should return build metadata when processes are present", func() {
					mkfile(t,
						`[[processes]]`+"\n"+
							`type = "A-type"`+"\n"+
							`command = "A-cmd"`+"\n"+
							`[[processes]]`+"\n"+
							`type = "override-type"`+"\n"+
							`command = "A-cmd"`+"\n",
						filepath.Join(appDir, "launch-A-v1.toml"),
					)
					mkfile(t,
						`[[processes]]`+"\n"+
							`type = "B-type"`+"\n"+
							`command = "B-cmd"`+"\n"+
							`[[processes]]`+"\n"+
							`type = "override-type"`+"\n"+
							`command = "B-cmd"`+"\n",
						filepath.Join(appDir, "launch-B-v2.toml"),
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
						filepath.Join(appDir, "build-env-A-v1", "SOME_VAR"),
						filepath.Join(appDir, "build-env-B-v2", "SOME_VAR"),
					)
				})

				it("should provide environment variables", func() {
					if _, err := builder.Build(); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(rdfile(t, filepath.Join(appDir, "build-info-A-v1")),
						"TEST_ENV: Av1\n",
					); s != "" {
						t.Fatalf("Unexpected info:\n%s\n", s)
					}
					if s := cmp.Diff(rdfile(t, filepath.Join(appDir, "build-info-B-v2")),
						"TEST_ENV: Bv2\n",
					); s != "" {
						t.Fatalf("Unexpected info:\n%s\n", s)
					}
				})

				it("should set CNB_BUILDPACK_DIR", func() {
					if _, err := builder.Build(); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					bpsDir, err := filepath.Abs(builder.BuildpacksDir)
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(rdfile(t, filepath.Join(appDir, "build-env-cnb-buildpack-dir-A-v1")),
						filepath.Join(bpsDir, "A/v1"),
					); s != "" {
						t.Fatalf("Unexpected CNB_BUILDPACK_DIR:\n%s\n", s)
					}
					if s := cmp.Diff(rdfile(t, filepath.Join(appDir, "build-env-cnb-buildpack-dir-B-v2")),
						filepath.Join(bpsDir, "B/v2"),
					); s != "" {
						t.Fatalf("Unexpected CNB_BUILDPACK_DIR:\n%s\n", s)
					}
				})

				it("should connect stdout and stdin to the terminal", func() {
					if _, err := builder.Build(); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(cleanEndings(stdout.String()), "build out: A@v1\nbuild out: B@v2\n"); s != "" {
						t.Fatalf("Unexpected stdout:\n%s\n", s)
					}
					if s := cmp.Diff(cleanEndings(stderr.String()), "build err: A@v1\nbuild err: B@v2\n"); s != "" {
						t.Fatalf("Unexpected stderr:\n%s\n", s)
					}
				})

				it("should provide a subset of the build plan to each buildpack", func() {
					builder.Plan = lifecycle.BuildPlan{
						Entries: []lifecycle.BuildPlanEntry{
							{
								Providers: []lifecycle.Buildpack{
									{ID: "A", Version: "v1"},
									{ID: "B", Version: "v2"},
								},
								Requires: []lifecycle.Require{
									{Name: "dep1", Version: "v1"},
								},
							},
							{
								Providers: []lifecycle.Buildpack{
									{ID: "A", Version: "v1"},
									{ID: "B", Version: "v2"},
								},
								Requires: []lifecycle.Require{
									{Name: "dep1-next", Version: "v2"},
								},
							},
							{
								Providers: []lifecycle.Buildpack{
									{ID: "A", Version: "v1"},
									{ID: "B", Version: "v2"},
								},
								Requires: []lifecycle.Require{
									{Name: "dep1-replace", Version: "v3"},
								},
							},
							{
								Providers: []lifecycle.Buildpack{
									{ID: "B", Version: "v2"},
								},
								Requires: []lifecycle.Require{
									{Name: "dep2", Version: "v4"},
								},
							},
							{
								Providers: []lifecycle.Buildpack{
									{ID: "B", Version: "v2"},
								},
								Requires: []lifecycle.Require{
									{Name: "dep2-next", Version: "v5"},
								},
							},
							{
								Providers: []lifecycle.Buildpack{
									{ID: "B", Version: "v2"},
								},
								Requires: []lifecycle.Require{
									{Name: "dep2-replace", Version: "v6"},
								},
							},
						},
					}

					mkfile(t,
						"[[entries]]\n"+
							`name = "dep1"`+"\n"+
							`version = "v1"`+"\n"+
							"[[entries]]\n"+
							`name = "dep1-replace"`+"\n"+
							`version = "v7"`+"\n",
						filepath.Join(appDir, "build-plan-out-A-v1.toml"),
					)
					mkfile(t,
						"[[entries]]\n"+
							`name = "dep1-next"`+"\n"+
							`version = "v9"`+"\n"+
							"[[entries]]\n"+
							`name = "dep2"`+"\n"+
							`version = "v4"`+"\n"+
							"[[entries]]\n"+
							`name = "dep2-replace"`+"\n"+
							`version = "v8"`+"\n",
						filepath.Join(appDir, "build-plan-out-B-v2.toml"),
					)
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
						BOM: []lifecycle.BOMEntry{
							{
								Require:   lifecycle.Require{Name: "dep1", Version: "v1"},
								Buildpack: lifecycle.Buildpack{ID: "A", Version: "v1"},
							},
							{
								Require:   lifecycle.Require{Name: "dep1-replace", Version: "v7"},
								Buildpack: lifecycle.Buildpack{ID: "A", Version: "v1"},
							},
							{
								Require:   lifecycle.Require{Name: "dep1-next", Version: "v9"},
								Buildpack: lifecycle.Buildpack{ID: "B", Version: "v2"},
							},
							{
								Require:   lifecycle.Require{Name: "dep2", Version: "v4"},
								Buildpack: lifecycle.Buildpack{ID: "B", Version: "v2"},
							},
							{
								Require:   lifecycle.Require{Name: "dep2-replace", Version: "v8"},
								Buildpack: lifecycle.Buildpack{ID: "B", Version: "v2"},
							},
						},
					}); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}

					testPlan(t,
						[]lifecycle.Require{
							{Name: "dep1", Version: "v1"},
							{Name: "dep1-next", Version: "v2"},
							{Name: "dep1-replace", Version: "v3"},
						},
						filepath.Join(appDir, "build-plan-in-A-v1.toml"),
					)

					testPlan(t,
						[]lifecycle.Require{
							{Name: "dep1-next", Version: "v2"},
							{Name: "dep2", Version: "v4"},
							{Name: "dep2-next", Version: "v5"},
							{Name: "dep2-replace", Version: "v6"},
						},
						filepath.Join(appDir, "build-plan-in-B-v2.toml"),
					)
				})
			})

			when("building succeeds with a clear env", func() {
				it.Before(func() {
					env.EXPECT().List().Return(append(os.Environ(), "TEST_ENV=cleared"))
					env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=with-platform"), nil)
					builder.Group.Group[0].Version = "v1.clear"
				})

				it("should not apply user-provided env vars", func() {
					if _, err := builder.Build(); err != nil {
						t.Fatalf("Error: %s\n", err)
					}
					if s := cmp.Diff(rdfile(t, filepath.Join(appDir, "build-info-A-v1.clear")),
						"TEST_ENV: cleared\n",
					); s != "" {
						t.Fatalf("Unexpected info:\n%s\n", s)
					}
					if s := cmp.Diff(rdfile(t, filepath.Join(appDir, "build-info-B-v2")),
						"TEST_ENV: with-platform\n",
					); s != "" {
						t.Fatalf("Unexpected info:\n%s\n", s)
					}
				})

				it("should set CNB_BUILDPACK_DIR", func() {
					if _, err := builder.Build(); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					bpsDir, err := filepath.Abs(builder.BuildpacksDir)
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(rdfile(t, filepath.Join(appDir, "build-env-cnb-buildpack-dir-A-v1.clear")),
						filepath.Join(bpsDir, "A/v1.clear"),
					); s != "" {
						t.Fatalf("Unexpected CNB_BUILDPACK_DIR:\n%s\n", s)
					}
					if s := cmp.Diff(rdfile(t, filepath.Join(appDir, "build-env-cnb-buildpack-dir-B-v2")),
						filepath.Join(bpsDir, "B/v2"),
					); s != "" {
						t.Fatalf("Unexpected CNB_BUILDPACK_DIR:\n%s\n", s)
					}
				})
			})

			when("building fails", func() {
				it("should error when layer directories cannot be created", func() {
					mkfile(t, "some-data", filepath.Join(layersDir, "A"))
					_, err := builder.Build()
					if _, ok := err.(*os.PathError); !ok {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				it("should error when the provided build plan is invalid", func() {
					builder.Plan = lifecycle.BuildPlan{
						Entries: []lifecycle.BuildPlanEntry{{
							Providers: []lifecycle.Buildpack{{ID: "A", Version: "v1"}},
							Requires: []lifecycle.Require{{
								Metadata: map[string]interface{}{"a": map[int64]int64{1: 2}},
							}},
						}}}
					if _, err := builder.Build(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "toml") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				it("should error when any build plan entry is invalid", func() {
					env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
					mkfile(t, "bad-key", filepath.Join(appDir, "build-plan-out-A-v1.toml"))
					if _, err := builder.Build(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "key") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				it("should error when the env cannot be found", func() {
					env.EXPECT().WithPlatform(platformDir).Return(nil, errors.New("some error"))
					if _, err := builder.Build(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "some error") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				it("should error when the command fails", func() {
					env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
					if err := os.RemoveAll(platformDir); err != nil {
						t.Fatalf("Error: %s\n", err)
					}
					_, err := builder.Build()
					if _, ok := err.(*exec.ExitError); !ok {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				when("modifying the env fails", func() {
					var appendErr error

					it.Before(func() {
						appendErr = errors.New("some error")
					})

					each(it, []func(){
						func() {
							env.EXPECT().AddRootDir(gomock.Any()).Return(appendErr)
						},
						func() {
							env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
							env.EXPECT().AddRootDir(gomock.Any()).Return(appendErr)
						},
						func() {
							env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
							env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
							env.EXPECT().AddEnvDir(gomock.Any()).Return(appendErr)
						},
						func() {
							env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
							env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
							env.EXPECT().AddEnvDir(gomock.Any()).Return(nil)
							env.EXPECT().AddEnvDir(gomock.Any()).Return(appendErr)
						},
					}, "should error", func() {
						env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
						mkdir(t,
							filepath.Join(appDir, "layers-A-v1", "layer1"),
							filepath.Join(appDir, "layers-A-v1", "layer2"),
						)
						mkfile(t, "build = true",
							filepath.Join(appDir, "layers-A-v1", "layer1.toml"),
							filepath.Join(appDir, "layers-A-v1", "layer2.toml"),
						)
						if _, err := builder.Build(); err != appendErr {
							t.Fatalf("Incorrect error: %s\n", err)
						}
					})
				})

				it("should error when launch.toml is not writable", func() {
					env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
					mkdir(t, filepath.Join(layersDir, "A", "launch.toml"))
					if _, err := builder.Build(); err == nil {
						t.Fatal("Expected error")
					}
				})
			})
		}
	})

	when("not taking snapshots", func() {
		it.Before(func() {
			builder = createBuilder(t, tmpDir, env, outLog, errLog)
			appDir = builder.AppDir
			platformDir = builder.PlatformDir
			layersDir = builder.LayersDir

			env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
			env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Bv2"), nil)
		})

		it("should the snapshots are not created", func() {
			if _, err := builder.Build(); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "A.tgz"))
			h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "B.tgz"))
		})
	})

	when("taking snapshots", func() {
		it.Before(func() {
			builder = createRootBuilder(t, tmpDir, env, outLog, errLog)
			appDir = builder.AppDir
			platformDir = builder.PlatformDir
			layersDir = builder.LayersDir

			env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
			env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Bv2"), nil)
		})

		it("should ensure the snapshots are created", func() {
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

					if !strings.HasSuffix(b.String(), "testdata/by-id/A/v1") {
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
	})
}

func mkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func mkfile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := ioutil.WriteFile(p, []byte(data), 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func tofile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0777)
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		if _, err := f.Write([]byte(data)); err != nil {
			f.Close()
			t.Fatalf("Error: %s\n", err)
		}
		f.Close()
	}
}

func cleanEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func rdfile(t *testing.T, path string) string {
	t.Helper()
	out, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Error: %s\n", err)
	}
	return cleanEndings(string(out))
}

func testExists(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func testPlan(t *testing.T, plan []lifecycle.Require, paths ...string) {
	t.Helper()
	for _, p := range paths {
		var c struct {
			Entries []lifecycle.Require `toml:"entries"`
		}
		if _, err := toml.DecodeFile(p, &c); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		if s := cmp.Diff(c.Entries, plan); s != "" {
			t.Fatalf("Unexpected plan:\n%s\n", s)
		}
	}
}

func each(it spec.S, befores []func(), text string, f func()) {
	for i := range befores {
		before := befores[i]
		it(fmt.Sprintf("%s #%d", text, i), func() { before(); f() })
	}
}

func createBuilder(t *testing.T, tmpDir string, env lifecycle.BuildEnv, outLog, errLog *log.Logger) *lifecycle.Builder {
	platformDir := filepath.Join(tmpDir, "platform")
	layersDir := filepath.Join(tmpDir, "launch")
	appDir := filepath.Join(layersDir, "app")
	mkdir(t, appDir, layersDir, filepath.Join(platformDir, "env"))

	buildpacksDir := filepath.Join("testdata", "by-id")

	return &lifecycle.Builder{
		AppDir:        appDir,
		LayersDir:     layersDir,
		PlatformDir:   platformDir,
		BuildpacksDir: buildpacksDir,
		Env:           env,
		Group: lifecycle.BuildpackGroup{
			Group: []lifecycle.Buildpack{
				{ID: "A", Version: "v1"},
				{ID: "B", Version: "v2"},
			},
		},
		Out:         outLog,
		Err:         errLog,
		Snapshotter: &lifecycle.NoopSnapshotter{},
	}
}

func createRootBuilder(t *testing.T, tmpDir string, env lifecycle.BuildEnv, outLog, errLog *log.Logger) *lifecycle.Builder {
	platformDir := filepath.Join(tmpDir, "platform")
	layersDir := filepath.Join(tmpDir, "launch")
	appDir := filepath.Join(layersDir, "app")
	mkdir(t, appDir, layersDir, filepath.Join(platformDir, "env"))

	buildpacksDir := filepath.Join("testdata", "by-id")

	snapshotter, err := snapshot.NewKanikoSnapshotter(appDir)
	if err != nil {
		t.Fatalf("Error: %s\n", err)
	}

	return &lifecycle.Builder{
		AppDir:        appDir,
		LayersDir:     layersDir,
		PlatformDir:   platformDir,
		BuildpacksDir: buildpacksDir,
		Env:           env,
		Group: lifecycle.BuildpackGroup{
			Group: []lifecycle.Buildpack{
				{ID: "A", Version: "v1"},
				{ID: "B", Version: "v2"},
			},
		},
		Out:         outLog,
		Err:         errLog,
		Snapshotter: snapshotter,
	}
}
