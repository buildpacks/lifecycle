package platform_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestTargetData(t *testing.T) {
	spec.Run(t, "target_data", testTargetData)
}

func testTargetData(t *testing.T, when spec.G, it spec.S) {
	when(".TargetSatisfiedForBuild", func() {
		baseTarget := &files.TargetMetadata{OS: "Win95", Arch: "Pentium"}
		d := mockDetector{
			contents: "this is just test contents really",
			t:        t,
			HasFile:  false, // by default, don't use info from /etc/os-release for these tests
		}

		when("base image data", func() {
			when("has os and arch", func() {
				when("buildpack data", func() {
					when("has os and arch", func() {
						it("must match", func() {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}, &log.Logger{Handler: memory.New()}), true)
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: "Win98", Arch: baseTarget.Arch}, &log.Logger{Handler: memory.New()}), false)
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: "Pentium MMX"}, &log.Logger{Handler: memory.New()}), false)
						})
					})

					when("missing os and arch", func() {
						it("matches", func() {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: "", Arch: ""}, &log.Logger{Handler: memory.New()}), true)
						})
					})

					when("has distro information", func() {
						it("does not match", func() {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{
								OS:      baseTarget.OS,
								Arch:    baseTarget.Arch,
								Distros: []buildpack.OSDistro{{Name: "a", Version: "2"}},
							}, &log.Logger{Handler: memory.New()}), false)
						})

						when("/etc/os-release has information", func() {
							it("must match", func() {
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

				when("has arch variant", func() {
					baseTarget.ArchVariant = "some-arch-variant"

					when("buildpack data", func() {
						when("has arch variant", func() {
							it("must match", func() {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-arch-variant"}, &log.Logger{Handler: memory.New()}), true)
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-other-arch-variant"}, &log.Logger{Handler: memory.New()}), false)
							})
						})

						when("missing arch variant", func() {
							it("matches", func() {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}, &log.Logger{Handler: memory.New()}), true)
							})
						})
					})
				})

				when("has distro information", func() {
					baseTarget.Distro = &files.OSDistro{Name: "A", Version: "1"}

					when("buildpack data", func() {
						when("has distro information", func() {
							it("must match", func() {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, Distros: []buildpack.OSDistro{{Name: "B", Version: "2"}, {Name: "A", Version: "1"}}}, &log.Logger{Handler: memory.New()}), true)
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, Distros: []buildpack.OSDistro{{Name: "g", Version: "2"}, {Name: "B", Version: "2"}}}, &log.Logger{Handler: memory.New()}), false)
							})
						})

						when("missing distro information", func() {
							it("matches", func() {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(&d, baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}, &log.Logger{Handler: memory.New()}), true)
							})
						})
					})
				})
			})
		})
	})

	when(".GetTargetOSFromFileSystem", func() {
		it("populates appropriately", func() {
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

		it("doesn't populate if there's no file", func() {
			logr := &log.Logger{Handler: memory.New()}
			tm := files.TargetMetadata{}
			d := mockDetector{contents: "in unit tests 2.0 the users will generate the content but we'll serve them ads",
				t:       t,
				HasFile: false}
			platform.GetTargetOSFromFileSystem(&d, &tm, logr)
			h.AssertNil(t, tm.Distro)
		})

		it("doesn't populate if there's an error reading the file", func() {
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

	when(".EnvVarsFor", func() {
		it("returns the right thing", func() {
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

		it("returns the right thing from /etc/os-release", func() {
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

		it("does not return the wrong thing", func() {
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

		when("optional vars are empty", func() {
			it("omits them", func() {
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

	when(".TargetSatisfiedForRebase", func() {
		var baseTarget files.TargetMetadata
		when("orig image data", func() {
			when("has os and arch", func() {
				baseTarget = files.TargetMetadata{OS: "Win95", Arch: "Pentium"}

				when("new image data", func() {
					it("must match", func() {
						h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}), true)
						h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: "Win98", Arch: baseTarget.Arch}), false)
						h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: "Pentium MMX"}), false)
					})

					when("has extra information", func() {
						it("matches", func() {
							h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "MMX"}), true)
							h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{
								OS:     baseTarget.OS,
								Arch:   baseTarget.Arch,
								Distro: &files.OSDistro{Name: "a", Version: "2"},
							}), true)
						})
					})
				})

				when("has arch variant", func() {
					baseTarget.ArchVariant = "some-arch-variant"

					when("new image data", func() {
						when("has arch variant", func() {
							it("must match", func() {
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-arch-variant"}), true)
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-other-arch-variant"}), false)
							})
						})

						when("missing arch variant", func() {
							it("matches", func() {
								h.AssertEq(t, platform.TargetSatisfiedForRebase(baseTarget, files.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}), true)
							})
						})
					})
				})

				when("has distro information", func() {
					baseTarget.Distro = &files.OSDistro{Name: "A", Version: "1"}

					when("new image data", func() {
						when("has distro information", func() {
							it("must match", func() {
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

						when("missing distro information", func() {
							it("errors", func() {
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
