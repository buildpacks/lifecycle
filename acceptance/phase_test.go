package acceptance

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ih "github.com/buildpacks/imgutil/testhelpers"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/registry"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

type PhaseTest struct {
	containerBinaryPath    string
	phaseName              string
	testImageDockerContext string
	testImageRef           string
	targetDaemon           *targetDaemon   // TODO: make optional so that these helpers can be used by detect & build
	targetRegistry         *targetRegistry // TODO: make optional so that these helpers can be used by detect & build
}

type targetDaemon struct {
	os       string
	arch     string
	fixtures *daemonImageFixtures
}

type daemonImageFixtures struct {
	AppImage   string
	CacheImage string
	RunImage   string
}

type targetRegistry struct {
	authConfig      string
	dockerConfigDir string
	network         string
	fixtures        *regImageFixtures
	registry        *ih.DockerRegistry
}

type regImageFixtures struct {
	InaccessibleImage      string
	ReadOnlyAppImage       string
	ReadOnlyCacheImage     string
	ReadOnlyRunImage       string
	ReadWriteAppImage      string
	ReadWriteCacheImage    string
	ReadWriteOtherAppImage string
	SomeAppImage           string
	SomeCacheImage         string
}

func NewPhaseTest(t *testing.T, phaseName, testImageDockerContext string) *PhaseTest {
	return &PhaseTest{
		containerBinaryPath:    "/cnb/lifecycle/" + phaseName, // TODO: consider calling ctrPath here to make the tests more readable
		phaseName:              phaseName,
		targetDaemon:           newTargetDaemon(t),
		targetRegistry:         newTargetRegistry(t),
		testImageDockerContext: testImageDockerContext,
		testImageRef:           "lifecycle/acceptance/" + phaseName,
	}
}

func newTargetDaemon(t *testing.T) *targetDaemon {
	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)

	arch := info.Architecture
	if arch == "x86_64" {
		arch = "amd64"
	}
	if arch == "aarch64" {
		arch = "arm64"
	}

	return &targetDaemon{
		os:   info.OSType,
		arch: arch,
	}
}

func newTargetRegistry(t *testing.T) *targetRegistry {
	dockerConfigDir, err := ioutil.TempDir("", "test.docker.config.dir")
	h.AssertNil(t, err)

	sharedRegHandler := registry.New(registry.Logger(log.New(ioutil.Discard, "", log.Lshortfile)))

	return &targetRegistry{
		dockerConfigDir: dockerConfigDir,
		fixtures:        nil,
		registry: ih.NewDockerRegistry(
			ih.WithAuth(dockerConfigDir),
			ih.WithSharedHandler(sharedRegHandler),
			ih.WithImagePrivileges(),
		),
	}
}

func (p *PhaseTest) Start(t *testing.T) {
	p.targetDaemon.createFixtures(t)

	p.targetRegistry.start(t)
	containerDockerConfigDir := filepath.Join(p.testImageDockerContext, "container", "docker-config")
	h.AssertNil(t, os.RemoveAll(containerDockerConfigDir))
	h.AssertNil(t, os.MkdirAll(containerDockerConfigDir, 0755)) // TODO: check permissions
	h.RecursiveCopy(t, p.targetRegistry.dockerConfigDir, containerDockerConfigDir)

	containerBinaryDir := filepath.Join(p.testImageDockerContext, "container", "cnb", "lifecycle")
	h.MakeAndCopyLifecycle(t, p.targetDaemon.os, p.targetDaemon.arch, containerBinaryDir) // TODO: only run make once
	h.DockerBuild(t, p.testImageRef, p.testImageDockerContext)
}

func (p *PhaseTest) Stop(t *testing.T) {
	p.targetDaemon.removeFixtures(t)

	p.targetRegistry.stop(t)
	// remove images that were built locally before being pushed to test registry
	cleanupFixtures(t, *p.targetRegistry.fixtures)

	h.DockerImageRemove(t, p.testImageRef)
}

func (d *targetDaemon) createFixtures(t *testing.T) {
	var fixtures daemonImageFixtures

	appMeta := minifyMetadata(t, filepath.Join("testdata", "analyzer", "app_image_metadata.json"), platform.LayersMetadata{}) // TODO: make this configurable per-phase
	cacheMeta := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})

	fixtures.AppImage = "some-app-image-" + h.RandString(10)
	cmd := exec.Command(
		"docker",
		"build",
		"-t", fixtures.AppImage,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+appMeta,
		filepath.Join("testdata", "analyzer", "app-image"),
	) // #nosec G204
	h.Run(t, cmd)

	fixtures.CacheImage = "some-cache-image-" + h.RandString(10)
	cmd = exec.Command(
		"docker",
		"build",
		"-t", fixtures.CacheImage,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+cacheMeta,
		filepath.Join("testdata", "analyzer", "cache-image"),
	) // #nosec G204
	h.Run(t, cmd)

	fixtures.RunImage = "some-run-image-" + h.RandString(10)
	cmd = exec.Command(
		"docker",
		"build",
		"-t", fixtures.RunImage,
		"--build-arg", "fromImage="+containerBaseImage,
		filepath.Join("testdata", "analyzer", "cache-image"),
	) // #nosec G204
	h.Run(t, cmd)

	d.fixtures = &fixtures
}

func (d *targetDaemon) removeFixtures(t *testing.T) {
	cleanupFixtures(t, *d.fixtures)
}

func (r *targetRegistry) start(t *testing.T) {
	r.registry.Start(t)

	// if registry is listening on localhost, use host networking to allow containers to reach it
	r.network = "default"
	if r.registry.Host == "localhost" {
		r.network = "host"
	}

	// Save auth config
	os.Setenv("DOCKER_CONFIG", r.dockerConfigDir)
	var err error
	r.authConfig, err = auth.BuildEnvVar(authn.DefaultKeychain, r.registry.RepoName("some-repo")) // repo name doesn't matter
	h.AssertNil(t, err)

	r.createFixtures(t)
}

func (r *targetRegistry) createFixtures(t *testing.T) {
	var fixtures regImageFixtures

	appMeta := minifyMetadata(t, filepath.Join("testdata", "analyzer", "app_image_metadata.json"), platform.LayersMetadata{})
	cacheMeta := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})

	// With Permissions

	fixtures.InaccessibleImage = r.registry.SetInaccessible("inaccessible-image")

	someReadOnlyAppName := "some-read-only-app-image-" + h.RandString(10)
	fixtures.ReadOnlyAppImage = buildRegistryImage(
		t,
		someReadOnlyAppName,
		filepath.Join("testdata", "analyzer", "app-image"),
		r.registry,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+appMeta,
	)
	r.registry.SetReadOnly(someReadOnlyAppName)

	someReadOnlyCacheImage := "some-read-only-cache-image-" + h.RandString(10)
	fixtures.ReadOnlyCacheImage = buildRegistryImage(
		t,
		someReadOnlyCacheImage,
		filepath.Join("testdata", "analyzer", "cache-image"),
		r.registry,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+cacheMeta,
	)
	r.registry.SetReadOnly(someReadOnlyCacheImage)

	someRunImageName := "some-read-only-run-image-" + h.RandString(10)
	buildRegistryImage(
		t,
		someRunImageName,
		filepath.Join("testdata", "analyzer", "cache-image"),
		r.registry,
		"--build-arg", "fromImage="+"ubuntu:bionic", // TODO: fix
	)
	fixtures.ReadOnlyRunImage = r.registry.SetReadOnly(someRunImageName)

	readWriteAppName := "some-read-write-app-image-" + h.RandString(10)
	fixtures.ReadWriteAppImage = buildRegistryImage(
		t,
		readWriteAppName,
		filepath.Join("testdata", "analyzer", "app-image"),
		r.registry,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+appMeta,
	)
	r.registry.SetReadWrite(readWriteAppName)

	someReadWriteCacheName := "some-read-write-cache-image-" + h.RandString(10)
	fixtures.ReadWriteCacheImage = buildRegistryImage(
		t,
		someReadWriteCacheName,
		filepath.Join("testdata", "analyzer", "cache-image"),
		r.registry,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+cacheMeta,
	)
	r.registry.SetReadWrite(someReadWriteCacheName)

	readWriteOtherAppName := "some-other-read-write-app-image-" + h.RandString(10)
	fixtures.ReadWriteOtherAppImage = buildRegistryImage(
		t,
		readWriteOtherAppName,
		filepath.Join("testdata", "analyzer", "app-image"),
		r.registry,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+appMeta,
	)
	r.registry.SetReadWrite(readWriteOtherAppName)

	// Without Permissions

	fixtures.SomeAppImage = buildRegistryImage(
		t,
		"some-app-image-"+h.RandString(10),
		filepath.Join("testdata", "analyzer", "app-image"),
		r.registry,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+appMeta,
	)

	fixtures.SomeCacheImage = buildRegistryImage(
		t,
		"some-cache-image-"+h.RandString(10),
		filepath.Join("testdata", "analyzer", "cache-image"),
		r.registry,
		"--build-arg", "fromImage="+containerBaseImage,
		"--build-arg", "metadata="+cacheMeta,
	)

	r.fixtures = &fixtures
}

func (r *targetRegistry) stop(t *testing.T) {
	r.registry.Stop(t)
	os.RemoveAll(r.dockerConfigDir)
}

func cleanupFixtures(t *testing.T, fixtures interface{}) {
	v := reflect.ValueOf(fixtures)

	for i := 0; i < v.NumField(); i++ {
		imageName := fmt.Sprintf("%v", v.Field(i).Interface())
		if strings.Contains(imageName, "inaccessible") {
			continue
		}
		h.DockerImageRemove(t, imageName)
	}
}
