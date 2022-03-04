package platform_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	ih "github.com/buildpacks/imgutil/testhelpers"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRegistryHandler(t *testing.T) {
	spec.Run(t, "unit-registry-handler", testRegistryHandler, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRegistryHandler(t *testing.T, when spec.G, it spec.S) {
	const (
		imageReadOnly     = "some-read-only-image"
		imageReadWrite    = "some-read-write-image"
		imageInaccessible = "some-inaccessible-image"
	)

	var (
		registryValidator *platform.DefaultRegistryValidator
		dockerConfigDir   string
		registry          *ih.DockerRegistry
	)

	it.Before(func() {
		var err error
		dockerConfigDir, err = ioutil.TempDir("", "test.docker.config.dir")
		h.AssertNil(t, err)

		registry = ih.NewDockerRegistry(
			ih.WithAuth(dockerConfigDir),
			ih.WithImagePrivileges(),
		)
		registry.Start(t)

		os.Setenv("DOCKER_CONFIG", dockerConfigDir)
		keychain, err := auth.DefaultKeychain(registry.RepoName("some-image"))
		h.AssertNil(t, err)

		createFixtures(t, registry, imageReadOnly, imageReadWrite, imageInaccessible)
		registry.SetReadOnly(imageReadOnly)
		registry.SetReadWrite(imageReadWrite)
		registry.SetInaccessible(imageInaccessible)

		registryValidator = platform.NewRegistryValidator(keychain)
	})

	it.After(func() {
		registry.Stop(t)
		h.AssertNil(t, os.RemoveAll(dockerConfigDir))
		os.Unsetenv("DOCKER_CONFIG")
		removeFixtures(t, imageReadOnly, imageReadWrite, imageInaccessible)
	})

	when("ValidateReadAccess", func() {
		when("image is readable", func() {
			it("returns nil", func() {
				h.AssertNil(t, registryValidator.ValidateReadAccess([]string{registry.RepoName(imageReadOnly)}))
			})
		})

		when("image is not readable", func() {
			it("returns an error", func() {
				h.AssertNotNil(t, registryValidator.ValidateReadAccess([]string{registry.RepoName(imageInaccessible)}))
			})
		})
	})

	when("ValidateWriteAccess", func() {
		when("image is writable", func() {
			it("returns nil", func() {
				h.AssertNil(t, registryValidator.ValidateWriteAccess([]string{registry.RepoName(imageReadWrite)}))
			})
		})

		when("image is not writable", func() {
			it("returns an error", func() {
				h.AssertNotNil(t, registryValidator.ValidateWriteAccess([]string{registry.RepoName(imageReadOnly)}))
			})
		})
	})

}

func createFixtures(t *testing.T, registry *ih.DockerRegistry, imageNames ...string) {
	for _, imageName := range imageNames {
		buildRegistryImage(t, imageName, filepath.Join("testdata", "registry"), registry)
	}
}

func buildRegistryImage(t *testing.T, repoName, context string, registry *ih.DockerRegistry) string {
	// Build image
	regRepoName := registry.RepoName(repoName)
	h.DockerBuild(t, regRepoName, context)

	// Push image
	h.AssertNil(t, h.PushImage(h.DockerCli(t), regRepoName, registry.EncodedLabeledAuth()))

	// Return registry repo name
	return regRepoName
}

func removeFixtures(t *testing.T, imageNames ...string) {
	for _, imageName := range imageNames {
		_, _, _ = h.RunE(exec.Command("docker", "rmi", imageName)) // #nosec G204
	}
}
