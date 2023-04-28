package files_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAnalyzed(t *testing.T) {
	spec.Run(t, "Analyzed", testAnalyzed)
	spec.Run(t, "PopulateTarget", testPopulateTarget)
}

func testAnalyzed(t *testing.T, when spec.G, it spec.S) {
	when("analyzed.toml", func() {
		when("it is old it stays old", func() {
			it("serializes and deserializes", func() {
				amd := files.Analyzed{
					PreviousImage: &files.ImageIdentifier{Reference: "previous-img"},
					LayersMetadata: files.LayersMetadata{
						Stack: files.Stack{
							RunImage: files.RunImageForExport{Image: "imagine that"},
						},
					},
					RunImage: &files.RunImage{Reference: "some-ref"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, encoding.WriteTOML(f, amd))
				amd2, err := files.ReadAnalyzed(f, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, amd.PreviousImageRef(), amd2.PreviousImageRef())
				h.AssertEq(t, amd.LayersMetadata, amd2.LayersMetadata)
				h.AssertEq(t, amd.BuildImage, amd2.BuildImage)
			})

			it("serializes to the old format", func() {
				amd := files.Analyzed{
					PreviousImage: &files.ImageIdentifier{Reference: "previous-img"},
					LayersMetadata: files.LayersMetadata{
						Stack: files.Stack{
							RunImage: files.RunImageForExport{Image: "imagine that"},
						},
					},
					RunImage: &files.RunImage{Reference: "some-ref"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, encoding.WriteTOML(f, amd))
				contents, err := os.ReadFile(f)
				h.AssertNil(t, err)
				expectedContents := `[image]
  reference = "previous-img"

[metadata]
  [metadata.config]
    sha = ""
  [metadata.launcher]
    sha = ""
  [metadata.process-types]
    sha = ""
  [metadata.run-image]
    top-layer = ""
    reference = ""
  [metadata.stack]
    [metadata.stack.run-image]
      image = "imagine that"

[run-image]
  reference = "some-ref"
`
				h.AssertEq(t, string(contents), expectedContents)
			})
		})

		when("it is new it stays new", func() {
			it("serializes and deserializes", func() {
				amd := files.Analyzed{
					PreviousImage: &files.ImageIdentifier{Reference: "the image formerly known as prince"},
					RunImage: &files.RunImage{
						Reference:      "librarian",
						TargetMetadata: &files.TargetMetadata{OS: "os/2 warp", Arch: "486"},
					},
					BuildImage: &files.ImageIdentifier{Reference: "implementation"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, encoding.WriteTOML(f, amd))
				amd2, err := files.ReadAnalyzed(f, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, amd.PreviousImageRef(), amd2.PreviousImageRef())
				h.AssertEq(t, amd.LayersMetadata, amd2.LayersMetadata)
				h.AssertEq(t, amd.BuildImage, amd2.BuildImage)
			})
		})

		when("TargetMetadata#IsSatisfiedBy", func() {
			it("requires equality of OS and Arch", func() {
				d := files.TargetMetadata{OS: "Win95", Arch: "Pentium"}

				if d.IsSatisfiedBy(&buildpack.TargetMetadata{OS: "Win98", Arch: d.Arch}) {
					t.Fatal("TargetMetadata with different OS were equal")
				}
				if d.IsSatisfiedBy(&buildpack.TargetMetadata{OS: d.OS, Arch: "Pentium MMX"}) {
					t.Fatal("TargetMetadata with different Arch were equal")
				}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{OS: d.OS, Arch: d.Arch, ArchVariant: "MMX"}) {
					t.Fatal("blank arch variant was not treated as wildcard")
				}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{
					OS:            d.OS,
					Arch:          d.Arch,
					Distributions: []buildpack.OSDistribution{{Name: "a", Version: "2"}},
				}) {
					t.Fatal("blank distributions list was not treated as wildcard")
				}

				d.Distribution = &files.OSDistribution{Name: "A", Version: "1"}
				if d.IsSatisfiedBy(&buildpack.TargetMetadata{OS: d.OS, Arch: d.Arch, Distributions: []buildpack.OSDistribution{{Name: "g", Version: "2"}, {Name: "B", Version: "2"}}}) {
					t.Fatal("unsatisfactory distribution lists were treated as satisfying")
				}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{OS: d.OS, Arch: d.Arch, Distributions: []buildpack.OSDistribution{}}) {
					t.Fatal("blank distributions list was not treated as wildcard")
				}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{OS: d.OS, Arch: d.Arch, Distributions: []buildpack.OSDistribution{{Name: "B", Version: "2"}, {Name: "A", Version: "1"}}}) {
					t.Fatal("distributions list including target's distribution not recognized as satisfying")
				}
			})

			it("is cool with starry arches", func() {
				d := files.TargetMetadata{OS: "windows", Arch: "amd64"}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{OS: d.OS, Arch: "*"}) {
					t.Fatal("Arch wildcard should have been satisfied with whatever we gave it")
				}
			})

			it("is down with OS stars", func() {
				d := files.TargetMetadata{OS: "plan 9", Arch: "amd64"}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{OS: "*", Arch: d.Arch}) {
					t.Fatal("OS wildcard should have been satisfied by plan 9")
				}
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

func testPopulateTarget(t *testing.T, when spec.G, it spec.S) {
	when("the data is available", func() {
		it("populates appropriately", func() {
			logr := &log.Logger{Handler: memory.New()}
			tm := files.TargetMetadata{}
			d := mockDetector{contents: "this is just test contents really",
				t:       t,
				HasFile: true}
			files.PopulateTargetOSFromFileSystem(&d, &tm, logr)
			h.AssertEq(t, "opensesame", tm.Distribution.Name)
			h.AssertEq(t, "3.14", tm.Distribution.Version)
		})

		it("doesn't populate if there's no file", func() {
			logr := &log.Logger{Handler: memory.New()}
			tm := files.TargetMetadata{}
			d := mockDetector{contents: "in unit tests 2.0 the users will generate the content but we'll serve them ads",
				t:       t,
				HasFile: false}
			files.PopulateTargetOSFromFileSystem(&d, &tm, logr)
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
			files.PopulateTargetOSFromFileSystem(&d, &tm, logr)
			h.AssertNil(t, tm.Distribution)
		})
	})
}
