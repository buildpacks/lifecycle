package selective_test

import (
	"io/ioutil"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/internal/selective"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSelective(t *testing.T) {
	spec.Run(t, "Selective", testSelective, spec.Report(report.Terminal{}))
}

func testSelective(t *testing.T, when spec.G, it spec.S) {
	when("AppendImage", func() {
		var (
			testImage v1.Image
			tmpDir    string
		)

		it.Before(func() {
			testImageName := "busybox"
			if runtime.GOOS == "windows" {
				testImageName = "mcr.microsoft.com/windows/nanoserver:1809"
			}
			ref, authr, err := auth.ReferenceForRepoName(authn.DefaultKeychain, testImageName)
			h.AssertNil(t, err)
			testImage, err = remote.Image(ref, remote.WithAuth(authr))
			h.AssertNil(t, err)

			tmpDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)
		})

		it("appends an image to a path without any layers", func() {
			digest, err := testImage.Digest()
			h.AssertNil(t, err)
			layoutPath, err := selective.Write(filepath.Join(tmpDir, "some-image-index"), empty.Index)
			h.AssertNil(t, err)

			h.AssertNil(t, layoutPath.AppendImage(testImage))

			fis, err := ioutil.ReadDir(filepath.Join(tmpDir, "some-image-index", "blobs", "sha256"))
			h.AssertNil(t, err)
			h.AssertEq(t, len(fis), 2) // manifest, config
			foundImage, err := layoutPath.Image(digest)
			h.AssertNil(t, err)

			// found image satisfies v1.Image interface
			_, err = foundImage.MediaType()
			h.AssertNil(t, err)
			_, err = foundImage.Size()
			h.AssertNil(t, err)
			_, err = foundImage.ConfigName()
			h.AssertNil(t, err)
			configFile, err := foundImage.ConfigFile()
			h.AssertNil(t, err)
			_, err = foundImage.RawConfigFile()
			h.AssertNil(t, err)
			foundImageDigest, err := foundImage.Digest()
			h.AssertNil(t, err)
			h.AssertEq(t, foundImageDigest.String(), digest.String())
			_, err = foundImage.Manifest()
			h.AssertNil(t, err)
			_, err = foundImage.RawManifest()
			h.AssertNil(t, err)
			foundLayers, err := foundImage.Layers()
			h.AssertNil(t, err)
			h.AssertEq(t, len(foundLayers), 1)
			foundLayerDigest, err := foundLayers[0].Digest()
			h.AssertNil(t, err)
			foundLayer, err := foundImage.LayerByDigest(foundLayerDigest)
			h.AssertNil(t, err)
			h.AssertEq(t, len(configFile.RootFS.DiffIDs), 1)
			_, err = foundImage.LayerByDiffID(configFile.RootFS.DiffIDs[0])
			h.AssertNil(t, err)

			// found layers satisfy v1.Layer interface
			_, err = foundLayer.DiffID()
			h.AssertNotNil(t, err)
			h.AssertStringContains(t, err.Error(), "no such file or directory") // this information could be obtained from the config, but ggcr tries to open the layer when getting this value
			_, err = foundLayer.Compressed()
			h.AssertNotNil(t, err)
			_, err = foundLayer.Uncompressed()
			h.AssertNotNil(t, err)
			_, err = foundLayer.Size()
			h.AssertNil(t, err)
			_, err = foundLayer.MediaType()
			h.AssertNil(t, err)
		})
	})
}
