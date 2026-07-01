package buildpack_test

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestBpDescriptor(t *testing.T) {
	t.Run("TargetMetadata", func(t *testing.T) {
		t.Run("#String()", func(t *testing.T) {
			t.Run("there is a distribution", func(t *testing.T) {
				t.Run("prints the target", func(t *testing.T) {
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
			t.Run("there is no distribution", func(t *testing.T) {
				t.Run("prints the target", func(t *testing.T) {
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
	t.Run("#ReadBpDescriptor", func(t *testing.T) {
		t.Run("returns a buildpack descriptor", func(t *testing.T) {
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
		t.Run("reads new target fields", func(t *testing.T) {
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
		t.Run("translating stacks to targets", func(t *testing.T) {
			t.Run("older buildpacks", func(t *testing.T) {
				t.Run("there is only bionic", func(t *testing.T) {
					t.Run("creates a target", func(t *testing.T) {
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
				t.Run("there are multiple stacks", func(t *testing.T) {
					t.Run("does NOT create a target", func(t *testing.T) {
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
				t.Run("there is a wildcard stack", func(t *testing.T) {
					t.Run("creates a wildcard target", func(t *testing.T) {
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
			t.Run("newer buildpacks", func(t *testing.T) {
				t.Run("there is only bionic", func(t *testing.T) {
					t.Run("creates a target", func(t *testing.T) {
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
				t.Run("there are multiple stacks", func(t *testing.T) {
					t.Run("creates a target", func(t *testing.T) {
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
				t.Run("there is a wildcard stack", func(t *testing.T) {
					t.Run("creates a wildcard target", func(t *testing.T) {
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
		t.Run("does not translate non-special stack values", func(t *testing.T) {
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
		t.Run("does autodetect linux buildpacks from the bin dir contents", func(t *testing.T) {
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
	})
}
