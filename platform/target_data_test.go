package platform_test

import (
	"fmt"
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
		var baseTarget files.TargetMetadata
		when("base image data", func() {
			when("has os and arch", func() {
				baseTarget = files.TargetMetadata{OS: "Win95", Arch: "Pentium"}

				when("buildpack data", func() {
					when("has os and arch", func() {
						it("must match", func() {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}), true)
							h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: "Win98", Arch: baseTarget.Arch}), false)
							h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: "Pentium MMX"}), false)
						})
					})

					when("missing os and arch", func() {
						it("matches", func() {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: "", Arch: ""}), true)
						})
					})

					when("has extra information", func() {
						it("matches", func() {
							h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "MMX"}), true)
							h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{
								OS:            baseTarget.OS,
								Arch:          baseTarget.Arch,
								Distributions: []buildpack.OSDistribution{{Name: "a", Version: "2"}},
							}), true)
						})
					})
				})

				when("has arch variant", func() {
					baseTarget.ArchVariant = "some-arch-variant"

					when("buildpack data", func() {
						when("has arch variant", func() {
							it("must match", func() {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-arch-variant"}), true)
								h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, ArchVariant: "some-other-arch-variant"}), false)
							})
						})

						when("missing arch variant", func() {
							it("matches", func() {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}), true)
							})
						})
					})
				})

				when("has distro information", func() {
					baseTarget.Distribution = &files.OSDistribution{Name: "A", Version: "1"}

					when("buildpack data", func() {
						when("has distro information", func() {
							it("must match", func() {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, Distributions: []buildpack.OSDistribution{{Name: "B", Version: "2"}, {Name: "A", Version: "1"}}}), true)
								h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch, Distributions: []buildpack.OSDistribution{{Name: "g", Version: "2"}, {Name: "B", Version: "2"}}}), false)
							})
						})

						when("missing distro information", func() {
							it("matches", func() {
								h.AssertEq(t, platform.TargetSatisfiedForBuild(baseTarget, buildpack.TargetMetadata{OS: baseTarget.OS, Arch: baseTarget.Arch}), true)
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
			h.AssertEq(t, "opensesame", tm.Distribution.Name)
			h.AssertEq(t, "3.14", tm.Distribution.Version)
		})

		it("doesn't populate if there's no file", func() {
			logr := &log.Logger{Handler: memory.New()}
			tm := files.TargetMetadata{}
			d := mockDetector{contents: "in unit tests 2.0 the users will generate the content but we'll serve them ads",
				t:       t,
				HasFile: false}
			platform.GetTargetOSFromFileSystem(&d, &tm, logr)
			h.AssertNil(t, tm.Distribution)
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
			h.AssertNil(t, tm.Distribution)
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
