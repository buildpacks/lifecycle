package inputs_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	ih "github.com/buildpacks/imgutil/testhelpers"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/platform/inputs"
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
		registryHandler    *inputs.DefaultRegistryHandler
		dockerConfigDir    string
		registry           *ih.DockerRegistry
		containerBaseImage string
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

		if runtime.GOOS == "windows" {
			containerBaseImage = "mcr.microsoft.com/windows/nanoserver:1809"
		} else {
			containerBaseImage = "scratch"
		}
		createFixtures(t, registry, containerBaseImage, imageReadOnly, imageReadWrite, imageInaccessible)
		registry.SetReadOnly(imageReadOnly)
		registry.SetReadWrite(imageReadWrite)
		registry.SetInaccessible(imageInaccessible)

		registryHandler = inputs.NewRegistryHandler(keychain)
	})

	it.After(func() {
		registry.Stop(t)
		h.AssertNil(t, os.RemoveAll(dockerConfigDir))
		os.Unsetenv("DOCKER_CONFIG")
		removeFixtures(t, imageReadOnly, imageReadWrite, imageInaccessible)
	})

	when("EnsureReadAccess", func() {
		when("image is readable", func() {
			it("returns nil", func() {
				h.AssertNil(t, registryHandler.EnsureReadAccess([]string{registry.RepoName(imageReadOnly)}))
			})
		})

		when("image is not readable", func() {
			it("returns an error", func() {
				h.AssertNotNil(t, registryHandler.EnsureReadAccess([]string{registry.RepoName(imageInaccessible)}))
			})
		})
	})

	when("EnsureWriteAccess", func() {
		when("image is writable", func() {
			it("returns nil", func() {
				h.AssertNil(t, registryHandler.EnsureWriteAccess([]string{registry.RepoName(imageReadWrite)}))
			})
		})

		when("image is not writable", func() {
			it("returns an error", func() {
				h.AssertNotNil(t, registryHandler.EnsureWriteAccess([]string{registry.RepoName(imageReadOnly)}))
			})
		})
	})
}

func createFixtures(t *testing.T, registry *ih.DockerRegistry, baseImage string, imageNames ...string) {
	for _, imageName := range imageNames {
		buildRegistryImage(t, imageName, filepath.Join("testdata", "registry"), registry, "--build-arg", "base_image="+baseImage)
	}
}

func buildRegistryImage(t *testing.T, repoName, context string, registry *ih.DockerRegistry, buildArgs ...string) string {
	// Build image
	regRepoName := registry.RepoName(repoName)
	h.DockerBuild(t, regRepoName, context, h.WithArgs(buildArgs...))

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
