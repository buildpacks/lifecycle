package platform_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestTargetData(t *testing.T) {
	t.Run(".TargetSatisfiedForBuild", func(t *testing.T) {
		baseTarget := &files.TargetMetadata{OS: "Win95", Arch: "Pentium"}
		d := mockDetector{
			contents: "this is just test contents really",
			t:        t,
			HasFile:  false, // by default, don't use info from /etc/os-release for these tests
		}
		t.Run("base image data", func(t *testing.T) {
			t.Run("has os and arch", func(t *testing.T) {
				t.Run("buildpack data", func(t *testing.T) {
					t.Run("has os and arch", func(t *testing.T) {
						t.Run("must match", func(t *testing.T) {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}, &log.Logger{Handler: memory.New()}), true)
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: "Win98", Arch: baseTarget.Arch}, &log.Logger{Handler: memory.New()}), false)
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: "Pentium MMX"}, &log.Logger{Handler: memory.New()}), false)
						})
					})
					t.Run("missing os and arch", func(t *testing.T) {
						t.Run("matches", func(t *testing.T) {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: "", Arch: ""}, &log.Logger{Handler: memory.New()}), true)
						})
					})
					t.Run("has distro information", func(t *testing.T) {
						t.Run("does not match", func(t *testing.T) {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{
								OS:      baseTarget.OS,
								Arch:    baseTarget.Arch,
								Distros: []buildpack.OSDistro{{Name: "a", Version: "2"}},
							}, &log.Logger{Handler: memory.New()}), false)
						})
						t.Run("/etc/os-release has information", func(t *testing.T) {
							t.Run("must match", func(t *testing.T) {
								d := mockDetector{
									contents: "this is just test contents really",
									t:        t,
									HasFile:  true,
								}
								h.AssertEq(
									t,
									platform.TargetSatisfiedForBuild(
										&d,
										baseTarget,
										buildpack.TargetMetadata{
											OS:   baseTarget.OS,
											Arch: baseTarget.Arch,
											Distros: []buildpack.OSDistro{
												{Name: "opensesame", Version: "3.14"},
											},
										},
										&log.Logger{Handler: memory.New()},
									), true)
							})
						})
					})
				})
				t.Run("has arch variant", func(t *testing.T) {
					baseTarget.ArchVariant = "some-arch-variant"
					t.Run("buildpack data", func(t *testing.T) {
						t.Run("has arch variant", func(t *testing.T) {
							t.Run("must match", func(t *testing.T) {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-arch-variant"}, &log.Logger{Handler: memory.New()}), true)
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-other-arch-variant"}, &log.Logger{Handler: memory.New()}), false)
							})
						})
						t.Run("missing arch variant", func(t *testing.T) {
							t.Run("matches", func(t *testing.T) {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}, &log.Logger{Handler: memory.New()}), true)
							})
						})
					})
				})
				t.Run("has distro information", func(t *testing.T) {
					baseTarget.Distro = &files.OSDistro{Name: "A", Version: "1"}
					t.Run("buildpack data", func(t *testing.T) {
						t.Run("has distro information", func(t *testing.T) {
							t.Run("must match", func(t *testing.T) {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, Distros: []buildpack.OSDistro{{Name: "B", Version: "2"}, {Name: "A", Version: "1"}}}, &log.Logger{Handler: memory.New()}), true)
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, Distros: []buildpack.OSDistro{{Name: "g", Version: "2"}, {Name: "B", Version: "2"}}}, &log.Logger{Handler: memory.New()}), false)
							})
						})
						t.Run("missing distro information", func(t *testing.T) {
							t.Run("matches", func(t *testing.T) {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}, &log.Logger{Handler: memory.New()}), true)
							})
						})
					})
				})
			})
		})
	})
	t.Run(".GetTargetOSFromFileSystem", func(t *testing.T) {
		t.Run("populates appropriately", func(t *testing.T) {
			logr := &log.Logger{Handler: memory.New()}
			tm := files.TargetMetadata{}
			d := mockDetector{contents: "this is just test contents really",
				t:       t,
				HasFile: true}
			platform.GetTargetOSFromFileSystem(&d, &tm, logr)
			h.AssertEq(t, "linux", tm.OS)
			h.AssertEq(t, runtime.GOARCH, tm.Arch)
			h.AssertEq(t, "opensesame", tm.Distro.Name)
			h.AssertEq(t, "3.14", tm.Distro.Version)
		})
		t.Run("doesn't populate if there's no file", func(t *testing.T) {
			logr := &log.Logger{Handler: memory.New()}
			tm := files.TargetMetadata{}
			d := mockDetector{contents: "in unit tests 2.0 the users will generate the content but we'll serve them ads",
				t:       t,
				HasFile: false}
			platform.GetTargetOSFromFileSystem(&d, &tm, logr)
			h.AssertNil(t, tm.Distro)
		})
		t.Run("doesn't populate if there's an error reading the file", func(t *testing.T) {
			logr := &log.Logger{Handler: memory.New()}
			tm := files.TargetMetadata{}
			d := mockDetector{contents: "contentment is the greatest wealth",
				t:           t,
				HasFile:     true,
				ReadFileErr: fmt.Errorf("I'm sorry Dave, I don't even remember exactly what HAL says"),
			}
			platform.GetTargetOSFromFileSystem(&d, &tm, logr)
			h.AssertNil(t, tm.Distro)
		})
	})
	t.Run(".EnvVarsFor", func(t *testing.T) {
		t.Run("returns the right thing", func(t *testing.T) {
			tm := files.TargetMetadata{Arch: "pentium", ArchVariant: "mmx", ID: "my-id", OS: "linux", Distro: &files.OSDistro{Name: "nix", Version: "22.11"}}
			d := &mockDetector{
				contents: "this is just test contents really",
				t:        t,
				HasFile:  false,
			}
			observed := platform.EnvVarsFor(d, tm, &log.Logger{Handler: memory.New()})
			h.AssertContains(t, observed, "CNB_TARGET_ARCH="+tm.Arch)
			h.AssertContains(t, observed, "CNB_TARGET_ARCH_VARIANT="+tm.ArchVariant)
			h.AssertContains(t, observed, "CNB_TARGET_DISTRO_NAME="+tm.Distro.Name)
			h.AssertContains(t, observed, "CNB_TARGET_DISTRO_VERSION="+tm.Distro.Version)
			h.AssertContains(t, observed, "CNB_TARGET_OS="+tm.OS)
			h.AssertEq(t, len(observed), 5)
		})
		t.Run("returns the right thing from /etc/os-release", func(t *testing.T) {
			d := &mockDetector{
				contents: "this is just test contents really",
				t:        t,
				HasFile:  true,
			}
			tm := files.TargetMetadata{Arch: "pentium", ArchVariant: "mmx", ID: "my-id", OS: "linux", Distro: nil}
			observed := platform.EnvVarsFor(d, tm, &log.Logger{Handler: memory.New()})
			h.AssertContains(t, observed, "CNB_TARGET_ARCH="+tm.Arch)
			h.AssertContains(t, observed, "CNB_TARGET_ARCH_VARIANT="+tm.ArchVariant)
			h.AssertContains(t, observed, "CNB_TARGET_DISTRO_NAME=opensesame")
			h.AssertContains(t, observed, "CNB_TARGET_DISTRO_VERSION=3.14")
			h.AssertContains(t, observed, "CNB_TARGET_OS="+tm.OS)
			h.AssertEq(t, len(observed), 5)
		})
		t.Run("does not return the wrong thing", func(t *testing.T) {
			tm := files.TargetMetadata{Arch: "pentium", OS: "linux"}
			d := &mockDetector{
				contents: "this is just test contents really",
				t:        t,
				HasFile:  false,
			}
			observed := platform.EnvVarsFor(d, tm, &log.Logger{Handler: memory.New()})
			h.AssertContains(t, observed, "CNB_TARGET_ARCH="+tm.Arch)
			h.AssertContains(t, observed, "CNB_TARGET_OS="+tm.OS)
			h.AssertEq(t, len(observed), 2)
		})
		t.Run("optional vars are empty", func(t *testing.T) {
			t.Run("omits them", func(t *testing.T) {
				tm := files.TargetMetadata{
					// required
					OS:   "linux",
					Arch: "pentium",
					// optional
					ArchVariant: "",
					Distro:      &files.OSDistro{Name: "nix", Version: ""},
					ID:          "",
				}
				d := &mockDetector{
					contents: "this is just test contents really",
					t:        t,
					HasFile:  false,
				}
				observed := platform.EnvVarsFor(d, tm, &log.Logger{Handler: memory.New()})
				h.AssertEq(t, len(observed), 3)
			})
		})
	})
	t.Run(".TargetSatisfiedForRebase", func(t *testing.T) {
		var baseTarget files.TargetMetadata
		t.Run("orig image data", func(t *testing.T) {
			t.Run("has os and arch", func(t *testing.T) {
				baseTarget = files.TargetMetadata{OS: "Win95", Arch: "Pentium"}
				t.Run("new image data", func(t *testing.T) {
					t.Run("must match", func(t *testing.T) {
						h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}), true)
						h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: "Win98", Arch: baseTarget.Arch}), false)
						h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: "Pentium MMX"}), false)
					})
					t.Run("has extra information", func(t *testing.T) {
						t.Run("matches", func(t *testing.T) {
							h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "MMX"}), true)
							h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{
								OS:     baseTarget.OS,
								Arch:   baseTarget.Arch,
								Distro: &files.OSDistro{Name: "a", Version: "2"},
							}), true)
						})
					})
				})
				t.Run("has arch variant", func(t *testing.T) {
					baseTarget.ArchVariant = "some-arch-variant"
					t.Run("new image data", func(t *testing.T) {
						t.Run("has arch variant", func(t *testing.T) {
							t.Run("must match", func(t *testing.T) {
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-arch-variant"}), true)
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-other-arch-variant"}), false)
							})
						})
						t.Run("missing arch variant", func(t *testing.T) {
							t.Run("matches", func(t *testing.T) {
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}), true)
							})
						})
					})
				})
				t.Run("has distro information", func(t *testing.T) {
					baseTarget.Distro = &files.OSDistro{Name: "A", Version: "1"}
					t.Run("new image data", func(t *testing.T) {
						t.Run("has distro information", func(t *testing.T) {
							t.Run("must match", func(t *testing.T) {
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{
									OS:     baseTarget.OS,
									Arch:   baseTarget.Arch,
									Distro: &files.OSDistro{Name: "A", Version: "1"},
								}), true)
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{
									OS:     baseTarget.OS,
									Arch:   baseTarget.Arch,
									Distro: &files.OSDistro{Name: "B", Version: "2"},
								}), false)
							})
						})
						t.Run("missing distro information", func(t *testing.T) {
							t.Run("errors", func(t *testing.T) {
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}), false)
							})
						})
					})
				})
			})
		})
	})
}

type mockDetector struct {
	contents    string
	t           *testing.T
	HasFile     bool
	ReadFileErr error
}

func (d *mockDetector) HasSystemdFile() bool {
	return d.HasFile
}

func (d *mockDetector) ReadSystemdFile() (string, error) {
	return d.contents, d.ReadFileErr
}

func (d *mockDetector) GetInfo(osReleaseContents string) fsutil.OSInfo {
	h.AssertEq(d.t, osReleaseContents, d.contents)
	return fsutil.OSInfo{
		Name:    "opensesame",
		Version: "3.14",
	}
}

func (d *mockDetector) InfoOnce(_ llog.Logger) {}

func (d *mockDetector) StoredInfo() *fsutil.OSInfo {
	return nil
}
