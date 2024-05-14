package buildpack_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestBpDescriptor(t *testing.T) {
	spec.Run(t, "BpDescriptor", testBpDescriptor, spec.Report(report.Terminal{}))
}

func testBpDescriptor(t *testing.T, when spec.G, it spec.S) {
	when("TargetMetadata", func() {
		when("#String()", func() {
			when("there is a distribution", func() {
				it("prints the target", func() {
					tm := &buildpack.TargetMetadata{
						OS:          "some-os",
						Arch:        "some-arch",
						ArchVariant: "some-arch-variant",
						Distros: []buildpack.OSDistro{
							{
								Name:    "some-os-dist",
								Version: "some-os-dist-version",
							},
						},
					}
					h.AssertEq(t, tm.String(), `{"os":"some-os","arch":"some-arch","arch-variant":"some-arch-variant","distros":[{"name":"some-os-dist","version":"some-os-dist-version"}]}`)
				})
			})

			when("there is no distribution", func() {
				it("prints the target", func() {
					tm := &buildpack.TargetMetadata{
						OS:          "some-os",
						Arch:        "some-arch",
						ArchVariant: "some-arch-variant",
					}
					h.AssertEq(t, tm.String(), `{"os":"some-os","arch":"some-arch","arch-variant":"some-arch-variant"}`)
				})
			})
		})
	})

	when("#ReadBpDescriptor", func() {
		it("returns a buildpack descriptor", func() {
			path := filepath.Join("testdata", "buildpack", "by-id", "A", "v1", "buildpack.toml")
			descriptor, err := buildpack.ReadBpDescriptor(path)
			h.AssertNil(t, err)

			h.AssertEq(t, descriptor.WithAPI, "0.7")
			h.AssertEq(t, descriptor.Buildpack.ID, "A")
			h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack A")
			h.AssertEq(t, descriptor.Buildpack.Version, "v1")
			h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack A Homepage")
			h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
		})

		it("reads new target fields", func() {
			path := filepath.Join("testdata", "buildpack", "by-id", "D", "v1", "buildpack.toml")
			descriptor, err := buildpack.ReadBpDescriptor(path)
			h.AssertNil(t, err)
			// common sanity checks
			h.AssertEq(t, descriptor.WithAPI, "0.12")
			h.AssertEq(t, descriptor.Buildpack.ID, "D")
			h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack D")
			h.AssertEq(t, descriptor.Buildpack.Version, "v1")
			h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack D Homepage")
			h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
			// specific behaviors for this test
			h.AssertEq(t, len(descriptor.Targets), 1)
			h.AssertEq(t, descriptor.Targets[0].Arch, "IA64")
			h.AssertEq(t, descriptor.Targets[0].OS, "OpenVMS")
			h.AssertEq(t, descriptor.Targets[0].Distros[0].Name, "VSI OpenVMS")
			h.AssertEq(t, descriptor.Targets[0].Distros[0].Version, "V8.4-2L3")
		})

		when("translating stacks to targets", func() {
			when("older buildpacks", func() {
				when("there is only bionic", func() {
					it("creates a target", func() {
						path := filepath.Join("testdata", "buildpack", "by-id", "B", "v1", "buildpack.toml")
						descriptor, err := buildpack.ReadBpDescriptor(path)
						h.AssertNil(t, err)
						// common sanity checks
						h.AssertEq(t, descriptor.WithAPI, "0.7")
						h.AssertEq(t, descriptor.Buildpack.ID, "B")
						h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack B")
						h.AssertEq(t, descriptor.Buildpack.Version, "v1")
						h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack B Homepage")
						h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
						// specific behaviors for this test
						h.AssertEq(t, descriptor.Stacks[0].ID, "io.buildpacks.stacks.bionic")
						h.AssertEq(t, len(descriptor.Targets), 1)
						h.AssertEq(t, descriptor.Targets[0].Arch, "amd64")
						h.AssertEq(t, descriptor.Targets[0].OS, "linux")
						h.AssertEq(t, descriptor.Targets[0].Distros[0].Name, "ubuntu")
						h.AssertEq(t, descriptor.Targets[0].Distros[0].Version, "18.04")
					})
				})

				when("there are multiple stacks", func() {
					it("does NOT create a target", func() {
						path := filepath.Join("testdata", "buildpack", "by-id", "B", "v1.2", "buildpack.toml")
						descriptor, err := buildpack.ReadBpDescriptor(path)
						h.AssertNil(t, err)
						// common sanity checks
						h.AssertEq(t, descriptor.WithAPI, "0.7")
						h.AssertEq(t, descriptor.Buildpack.ID, "B")
						h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack B")
						h.AssertEq(t, descriptor.Buildpack.Version, "v1.2")
						h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack B Homepage")
						h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
						// specific behaviors for this test
						h.AssertEq(t, descriptor.Stacks[0].ID, "io.buildpacks.stacks.bionic")
						h.AssertEq(t, len(descriptor.Targets), 0)
					})
				})

				when("there is a wildcard stack", func() {
					it("creates a wildcard target", func() {
						path := filepath.Join("testdata", "buildpack", "by-id", "B", "v1.star", "buildpack.toml")
						descriptor, err := buildpack.ReadBpDescriptor(path)
						h.AssertNil(t, err)
						// common sanity checks
						h.AssertEq(t, descriptor.WithAPI, "0.7")
						h.AssertEq(t, descriptor.Buildpack.ID, "B")
						h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack B")
						h.AssertEq(t, descriptor.Buildpack.Version, "v1.star")
						h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack B Homepage")
						h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
						// specific behaviors for this test
						h.AssertEq(t, descriptor.Stacks[0].ID, "*")
						h.AssertEq(t, len(descriptor.Targets), 1)
						// a target that is completely empty will always match whatever is the base image target
						h.AssertEq(t, descriptor.Targets[0].Arch, "")
						h.AssertEq(t, descriptor.Targets[0].OS, "")
						h.AssertEq(t, descriptor.Targets[0].ArchVariant, "")
						h.AssertEq(t, len(descriptor.Targets[0].Distros), 0)
					})
				})
			})

			when("newer buildpacks", func() {
				when("there is only bionic", func() {
					it("creates a target", func() {
						path := filepath.Join("testdata", "buildpack", "by-id", "B", "v2", "buildpack.toml")
						descriptor, err := buildpack.ReadBpDescriptor(path)
						h.AssertNil(t, err)
						// common sanity checks
						h.AssertEq(t, descriptor.WithAPI, "0.12")
						h.AssertEq(t, descriptor.Buildpack.ID, "B")
						h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack B")
						h.AssertEq(t, descriptor.Buildpack.Version, "v2")
						h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack B Homepage")
						h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
						// specific behaviors for this test
						h.AssertEq(t, descriptor.Stacks[0].ID, "io.buildpacks.stacks.bionic")
						h.AssertEq(t, len(descriptor.Targets), 1)
						h.AssertEq(t, descriptor.Targets[0].Arch, "amd64")
						h.AssertEq(t, descriptor.Targets[0].OS, "linux")
						h.AssertEq(t, descriptor.Targets[0].Distros[0].Name, "ubuntu")
						h.AssertEq(t, descriptor.Targets[0].Distros[0].Version, "18.04")
					})
				})

				when("there are multiple stacks", func() {
					it("creates a target", func() {
						path := filepath.Join("testdata", "buildpack", "by-id", "B", "v2.2", "buildpack.toml")
						descriptor, err := buildpack.ReadBpDescriptor(path)
						h.AssertNil(t, err)
						// common sanity checks
						h.AssertEq(t, descriptor.WithAPI, "0.12")
						h.AssertEq(t, descriptor.Buildpack.ID, "B")
						h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack B")
						h.AssertEq(t, descriptor.Buildpack.Version, "v2.2")
						h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack B Homepage")
						h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
						// specific behaviors for this test
						h.AssertEq(t, descriptor.Stacks[0].ID, "io.buildpacks.stacks.bionic")
						h.AssertEq(t, len(descriptor.Targets), 1)
						h.AssertEq(t, descriptor.Targets[0].Arch, "amd64")
						h.AssertEq(t, descriptor.Targets[0].OS, "linux")
						h.AssertEq(t, descriptor.Targets[0].Distros[0].Name, "ubuntu")
						h.AssertEq(t, descriptor.Targets[0].Distros[0].Version, "18.04")
					})
				})

				when("there is a wildcard stack", func() {
					it("creates a wildcard target", func() {
						path := filepath.Join("testdata", "buildpack", "by-id", "B", "v2.star", "buildpack.toml")
						descriptor, err := buildpack.ReadBpDescriptor(path)
						h.AssertNil(t, err)
						// common sanity checks
						h.AssertEq(t, descriptor.WithAPI, "0.12")
						h.AssertEq(t, descriptor.Buildpack.ID, "B")
						h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack B")
						h.AssertEq(t, descriptor.Buildpack.Version, "v2.star")
						h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack B Homepage")
						h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
						// specific behaviors for this test
						h.AssertEq(t, descriptor.Stacks[0].ID, "*")
						h.AssertEq(t, len(descriptor.Targets), 1)
						// a target that is completely empty will always match whatever is the base image target
						h.AssertEq(t, descriptor.Targets[0].Arch, "")
						h.AssertEq(t, descriptor.Targets[0].OS, "")
						h.AssertEq(t, descriptor.Targets[0].ArchVariant, "")
						h.AssertEq(t, len(descriptor.Targets[0].Distros), 0)
					})
				})
			})
		})

		it("does not translate non-special stack values", func() {
			path := filepath.Join("testdata", "buildpack", "by-id", "C", "v1", "buildpack.toml")
			descriptor, err := buildpack.ReadBpDescriptor(path)
			h.AssertNil(t, err)
			// common sanity assertions
			h.AssertEq(t, descriptor.WithAPI, "0.12")
			h.AssertEq(t, descriptor.Buildpack.ID, "C")
			h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack C")
			h.AssertEq(t, descriptor.Buildpack.Version, "v1")
			h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack C Homepage")
			h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
			// specific behaviors for this test
			h.AssertEq(t, descriptor.Stacks[0].ID, "some.non-magic.value")
			h.AssertEq(t, len(descriptor.Targets), 0)
		})

		it("does autodetect linux buildpacks from the bin dir contents", func() {
			path := filepath.Join("testdata", "buildpack", "by-id", "C", "v2", "buildpack.toml")
			descriptor, err := buildpack.ReadBpDescriptor(path)
			h.AssertNil(t, err)
			// common sanity assertions
			h.AssertEq(t, descriptor.WithAPI, "0.12")
			h.AssertEq(t, descriptor.Buildpack.ID, "C")
			h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack C")
			h.AssertEq(t, descriptor.Buildpack.Version, "v2")
			h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack C Homepage")
			h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
			// specific behaviors for this test
			h.AssertEq(t, descriptor.Stacks[0].ID, "some.non-magic.value")
			h.AssertEq(t, len(descriptor.Targets), 1)
			h.AssertEq(t, len(descriptor.Targets), 1)
			h.AssertEq(t, descriptor.Targets[0].Arch, "")
			h.AssertEq(t, descriptor.Targets[0].OS, "linux")
			h.AssertEq(t, len(descriptor.Targets[0].Distros), 0)
		})

		it("detects windows/* if batch files are present and ignores linux", func() {
			path := filepath.Join("testdata", "buildpack", "by-id", "A", "v1", "buildpack.toml")
			descriptor, err := buildpack.ReadBpDescriptor(path)
			h.AssertNil(t, err)
			// common sanity assertions
			h.AssertEq(t, descriptor.WithAPI, "0.7")
			h.AssertEq(t, descriptor.Buildpack.ID, "A")
			h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack A")
			h.AssertEq(t, descriptor.Buildpack.Version, "v1")
			h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack A Homepage")
			h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
			// specific behaviors for this test
			h.AssertEq(t, len(descriptor.Targets), 2)
			h.AssertEq(t, descriptor.Targets[0].Arch, "")
			h.AssertEq(t, descriptor.Targets[0].OS, "windows")
			h.AssertEq(t, descriptor.Targets[1].Arch, "")
			h.AssertEq(t, descriptor.Targets[1].OS, "linux")
		})
	})
}
