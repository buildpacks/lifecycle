package cache_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack/layertypes"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/common"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestVolumeCache(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "VolumeCache", testVolumeCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testVolumeCache(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir       string
		volumeDir    string
		subject      *cache.VolumeCache
		backupDir    string
		stagingDir   string
		committedDir string
	)

	it.Before(func() {
		var err error

		tmpDir, err = ioutil.TempDir("", "lifecycle.cache.volume_cache")
		h.AssertNil(t, err)

		volumeDir = filepath.Join(tmpDir, "test_volume")
		h.AssertNil(t, os.MkdirAll(volumeDir, os.ModePerm))

		backupDir = filepath.Join(volumeDir, "committed-backup")
		stagingDir = filepath.Join(volumeDir, "staging")
		committedDir = filepath.Join(volumeDir, "committed")
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#NewVolumeCache", func() {
		it("returns an error when the volume path does not exist", func() {
			_, err := cache.NewVolumeCache(filepath.Join(tmpDir, "does_not_exist"))
			if err == nil {
				t.Fatal("expected NewVolumeCache to fail because volume path does not exist")
			}
		})

		when("staging already exists", func() {
			it.Before(func() {
				stagingPath := filepath.Join(volumeDir, "staging")
				h.AssertNil(t, os.MkdirAll(stagingPath, 0777))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(stagingPath, "some-layer.tar"), []byte("some data"), 0600))
			})

			it("clears staging", func() {
				var err error

				subject, err = cache.NewVolumeCache(volumeDir)
				h.AssertNil(t, err)

				_, err = os.Stat(filepath.Join(stagingDir, "some-layer.tar"))
				if err == nil {
					t.Fatal("expect NewVolumeCache to clear the staging dir")
				}
			})
		})

		when("staging does not exist", func() {
			it("creates staging dir", func() {
				var err error

				subject, err = cache.NewVolumeCache(volumeDir)
				h.AssertNil(t, err)

				_, err = os.Stat(stagingDir)
				h.AssertNil(t, err)
			})
		})

		when("committed does not exist", func() {
			it("creates committed dir", func() {
				var err error

				subject, err = cache.NewVolumeCache(volumeDir)
				h.AssertNil(t, err)

				_, err = os.Stat(committedDir)
				h.AssertNil(t, err)
			})
		})

		when("backup dir already exists", func() {
			it.Before(func() {
				h.AssertNil(t, os.MkdirAll(backupDir, 0777))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(backupDir, "some-layer.tar"), []byte("some data"), 0600))
			})

			it("clears the backup dir", func() {
				var err error

				subject, err = cache.NewVolumeCache(volumeDir)
				h.AssertNil(t, err)

				_, err = os.Stat(filepath.Join(backupDir, "some-layer.tar"))
				if err == nil {
					t.Fatal("expect NewVolumeCache to clear the staging dir")
				}
			})
		})
	})

	when("VolumeCache", func() {
		it.Before(func() {
			var err error

			subject, err = cache.NewVolumeCache(volumeDir)
			h.AssertNil(t, err)
		})

		when("#Name", func() {
			it("returns the volume path", func() {
				h.AssertEq(t, subject.Name(), volumeDir)
			})
		})

		when("#RetrieveMetadata", func() {
			when("volume contains valid metadata", func() {
				it.Before(func() {
					content := []byte(`{"buildpacks": [{"key": "bp.id", "version": "1.2.3", "layers": {"some-layer": {"sha": "some-sha", "data": "some-data", "build": true, "launch": false, "cache": true}}}]}`)
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), content, 0600))
				})

				it("returns the metadata", func() {
					expected := platform.CacheMetadata{
						Buildpacks: []common.BuildpackLayersMetadata{{
							ID:      "bp.id",
							Version: "1.2.3",
							Layers: map[string]common.BuildpackLayerMetadata{
								"some-layer": {
									LayerMetadata: common.LayerMetadata{
										SHA: "some-sha",
									},
									LayerMetadataFile: layertypes.LayerMetadataFile{
										Data:   "some-data",
										Build:  true,
										Launch: false,
										Cache:  true,
									},
								},
							},
						}},
					}

					meta, err := subject.RetrieveMetadata()
					h.AssertNil(t, err)
					h.AssertEq(t, meta, expected)
				})
			})

			when("volume contains invalid metadata", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), []byte("garbage"), 0600))
				})

				it("returns empty metadata", func() {
					meta, err := subject.RetrieveMetadata()
					h.AssertNil(t, err)
					h.AssertEq(t, len(meta.Buildpacks), 0)
				})
			})

			when("volume is empty", func() {
				it("returns empty metadata", func() {
					meta, err := subject.RetrieveMetadata()
					h.AssertNil(t, err)
					h.AssertEq(t, len(meta.Buildpacks), 0)
				})
			})
		})

		when("#RetrieveLayer", func() {
			when("layer exists", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "some_sha.tar"), []byte("dummy data"), 0600))
				})

				it("returns the layer's reader", func() {
					rc, err := subject.RetrieveLayer("some_sha")
					h.AssertNil(t, err)

					bytes, err := ioutil.ReadAll(rc)
					h.AssertNil(t, err)
					h.AssertEq(t, string(bytes), "dummy data")
				})
			})

			when("layer does not exist", func() {
				it("returns an error", func() {
					_, err := subject.RetrieveLayer("some_nonexistent_sha")
					h.AssertError(t, err, "layer with SHA 'some_nonexistent_sha' not found")
				})
			})
		})

		when("#RetrieveLayerFile", func() {
			when("layer exists", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "some_sha.tar"), []byte("dummy data"), 0600))
				})

				it("returns the layer's reader", func() {
					layerPath, err := subject.RetrieveLayerFile("some_sha")
					h.AssertNil(t, err)

					bytes, err := ioutil.ReadFile(layerPath)
					h.AssertNil(t, err)
					h.AssertEq(t, string(bytes), "dummy data")
				})
			})

			when("layer does not exist", func() {
				it("returns an error", func() {
					_, err := subject.RetrieveLayerFile("some_nonexistent_sha")
					h.AssertError(t, err, "layer with SHA 'some_nonexistent_sha' not found")
				})
			})
		})

		when("#Commit", func() {
			it("should clear the staging dir", func() {
				layerTarPath := filepath.Join(stagingDir, "some-layer.tar")
				h.AssertNil(t, ioutil.WriteFile(layerTarPath, []byte("some data"), 0600))

				err := subject.Commit()
				h.AssertNil(t, err)

				_, err = os.Stat(layerTarPath)
				if err == nil {
					t.Fatal("expected staging dir to have been cleared")
				}
			})

			when("#SetMetadata", func() {
				var newMetadata platform.CacheMetadata

				it.Before(func() {
					previousContents := []byte(`{"buildpacks": [{"key": "old.bp.id"}]}`)
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), previousContents, 0600))

					newMetadata = platform.CacheMetadata{
						Buildpacks: []common.BuildpackLayersMetadata{{
							ID: "new.bp.id",
						}},
					}
				})

				when("set then commit", func() {
					it("retrieve returns the newly set metadata", func() {
						h.AssertNil(t, subject.SetMetadata(newMetadata))

						err := subject.Commit()
						h.AssertNil(t, err)

						retrievedMetadata, err := subject.RetrieveMetadata()
						h.AssertNil(t, err)
						h.AssertEq(t, retrievedMetadata, newMetadata)
					})
				})

				when("set after commit", func() {
					it("retrieve returns the newly set metadata", func() {
						err := subject.Commit()
						h.AssertNil(t, err)

						h.AssertError(t, subject.SetMetadata(newMetadata), "cache cannot be modified after commit")
					})
				})

				when("set without commit", func() {
					it("retrieve returns the previous metadata", func() {
						previousMetadata := platform.CacheMetadata{
							Buildpacks: []common.BuildpackLayersMetadata{{
								ID: "old.bp.id",
							}},
						}

						h.AssertNil(t, subject.SetMetadata(newMetadata))

						retrievedMetadata, err := subject.RetrieveMetadata()
						h.AssertNil(t, err)
						h.AssertEq(t, retrievedMetadata, previousMetadata)
					})
				})
			})

			when("#AddLayerFile", func() {
				var tarPath string

				it.Before(func() {
					tarPath = filepath.Join(tmpDir, "some-layer.tar")
					h.AssertNil(t, ioutil.WriteFile(tarPath, []byte("dummy data"), 0600))
				})

				when("add then commit", func() {
					it("retrieve returns newly added layer", func() {
						h.AssertNil(t, subject.AddLayerFile(tarPath, "some_sha"))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer("some_sha")
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

				when("add after commit", func() {
					it("retrieve returns the newly set metadata", func() {
						err := subject.Commit()
						h.AssertNil(t, err)

						h.AssertError(t, subject.AddLayerFile(tarPath, "some_sha"), "cache cannot be modified after commit")
					})
				})

				when("add without commit", func() {
					it("retrieve returns not found error", func() {
						h.AssertNil(t, subject.AddLayerFile(tarPath, "some_sha"))

						_, err := subject.RetrieveLayer("some_sha")
						h.AssertError(t, err, "layer with SHA 'some_sha' not found")
					})
				})

				when("a layer with the same sha already exists", func() {
					it.Before(func() {
						existingLayerTar, err := ioutil.TempFile("", "*.tar")
						h.AssertNil(t, err)
						h.AssertNil(t, ioutil.WriteFile(existingLayerTar.Name(), []byte("existing data"), 0600))
						h.AssertNil(t, subject.AddLayerFile(existingLayerTar.Name(), "some_sha"))
					})

					it("does nothing", func() {
						h.AssertNil(t, subject.AddLayerFile(tarPath, "some_sha"))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer("some_sha")
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "existing data")
					})
				})
			})

			when("#AddLayer", func() {
				var (
					layerReader io.ReadCloser
					layerSha    string
					layerData   []byte
				)

				it.Before(func() {
					var (
						layerPath string
						err       error
					)
					layerPath, layerSha, layerData = h.RandomLayer(t, tmpDir)
					layerReader, err = os.Open(layerPath)
					h.AssertNil(t, err)
				})

				when("add then commit", func() {
					it("retrieve returns newly added layer", func() {
						h.AssertNil(t, subject.AddLayer(layerReader, layerSha))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer(layerSha)
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, bytes, layerData)
					})
				})

				when("add after commit", func() {
					it("retrieve returns the newly set metadata", func() {
						err := subject.Commit()
						h.AssertNil(t, err)

						h.AssertError(t, subject.AddLayer(layerReader, layerSha), "cache cannot be modified after commit")
					})
				})

				when("add without commit", func() {
					it("retrieve returns not found error", func() {
						h.AssertNil(t, subject.AddLayer(layerReader, layerSha))

						_, err := subject.RetrieveLayer(layerSha)
						h.AssertError(t, err, fmt.Sprintf("layer with SHA '%s' not found", layerSha))
					})
				})

				when("a layer with the same sha already exists", func() {
					it.Before(func() {
						existingLayerTar, err := ioutil.TempFile("", "*.tar")
						h.AssertNil(t, err)
						h.AssertNil(t, ioutil.WriteFile(existingLayerTar.Name(), layerData, 0600))
						h.AssertNil(t, subject.AddLayerFile(existingLayerTar.Name(), layerSha))
					})

					it("succeeds", func() {
						h.AssertNil(t, subject.AddLayer(layerReader, layerSha))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer(layerSha)
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, bytes, layerData)
					})
				})
			})

			when("#ReuseLayer", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "some_sha.tar"), []byte("dummy data"), 0600))
				})

				when("reuse then commit", func() {
					it("retrieve returns the reused layer", func() {
						h.AssertNil(t, subject.ReuseLayer("some_sha"))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer("some_sha")
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

				when("reuse after commit", func() {
					it("retrieve returns the newly set metadata", func() {
						err := subject.Commit()
						h.AssertNil(t, err)

						h.AssertError(t, subject.ReuseLayer("some_sha"), "cache cannot be modified after commit")
					})
				})

				when("reuse without commit", func() {
					it("retrieve returns the previous layer", func() {
						h.AssertNil(t, subject.ReuseLayer("some_sha"))

						rc, err := subject.RetrieveLayer("some_sha")
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

				when("a layer with the same sha already exists", func() {
					it.Before(func() {
						tarPath := filepath.Join(tmpDir, "some-layer.tar")
						h.AssertNil(t, ioutil.WriteFile(tarPath, []byte("existing data"), 0600))
						h.AssertNil(t, subject.AddLayerFile(tarPath, "some_sha"))
					})

					it("does nothing", func() {
						h.AssertNil(t, subject.ReuseLayer("some_sha"))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer("some_sha")
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "existing data")
					})
				})
			})

			when("attempting to commit more than once", func() {
				it("should fail", func() {
					err := subject.Commit()
					h.AssertNil(t, err)

					err = subject.Commit()
					h.AssertError(t, err, "cache cannot be modified after commit")
				})
			})
		})
	})
}
