package cache_test

import (
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cache"

	h "github.com/buildpack/lifecycle/testhelpers"
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
		emptyLogger  *log.Logger
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

		emptyLogger = log.New(ioutil.Discard, "", 0)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#NewVolumeCache", func() {
		it("returns an error when the volume path does not exist", func() {
			_, err := cache.NewVolumeCache(emptyLogger, filepath.Join(tmpDir, "does_not_exist"))
			if err == nil {
				t.Fatal("expected NewVolumeCache to fail because volume path does not exist")
			}
		})

		when("staging already exists", func() {
			it.Before(func() {
				stagingPath := filepath.Join(volumeDir, "staging")
				h.AssertNil(t, os.MkdirAll(stagingPath, 0777))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(stagingPath, "some-layer.tar"), []byte("some data"), 0666))
			})

			it("clears staging", func() {
				var err error

				subject, err = cache.NewVolumeCache(emptyLogger, volumeDir)
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

				subject, err = cache.NewVolumeCache(emptyLogger, volumeDir)
				h.AssertNil(t, err)

				_, err = os.Stat(stagingDir)
				h.AssertNil(t, err)
			})
		})

		when("committed does not exist", func() {
			it("creates committed dir", func() {
				var err error

				subject, err = cache.NewVolumeCache(emptyLogger, volumeDir)
				h.AssertNil(t, err)

				_, err = os.Stat(committedDir)
				h.AssertNil(t, err)
			})
		})

		when("backup dir already exists", func() {
			it.Before(func() {
				h.AssertNil(t, os.MkdirAll(backupDir, 0777))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(backupDir, "some-layer.tar"), []byte("some data"), 0666))
			})

			it("clears the backup dir", func() {
				var err error

				subject, err = cache.NewVolumeCache(emptyLogger, volumeDir)
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

			subject, err = cache.NewVolumeCache(emptyLogger, volumeDir)
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
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), content, 0666))
				})

				it("returns the metadata and found true", func() {
					expected := lifecycle.CacheMetadata{
						Buildpacks: []lifecycle.BuildpackMetadata{{
							ID:      "bp.id",
							Version: "1.2.3",
							Layers: map[string]lifecycle.LayerMetadata{
								"some-layer": {
									SHA:    "some-sha",
									Data:   "some-data",
									Build:  true,
									Launch: false,
									Cache:  true,
								},
							},
						}},
					}

					metadata, found, err := subject.RetrieveMetadata()
					h.AssertNil(t, err)
					h.AssertEq(t, found, true)
					h.AssertEq(t, metadata, expected)
				})
			})

			when("volume contains invalid metadata", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), []byte("garbage"), 0666))
				})

				it("returns empty metadata and found false", func() {
					metadata, found, err := subject.RetrieveMetadata()
					h.AssertNil(t, err)
					h.AssertEq(t, found, false)
					h.AssertEq(t, len(metadata.Buildpacks), 0)
				})
			})

			when("volume is empty", func() {
				it("returns empty metadata and found false", func() {
					metadata, found, err := subject.RetrieveMetadata()
					h.AssertNil(t, err)
					h.AssertEq(t, found, false)
					h.AssertEq(t, len(metadata.Buildpacks), 0)
				})
			})
		})

		when("#RetrieveLayer", func() {
			when("layer exists", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "some_sha.tar"), []byte("dummy data"), 0666))
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

		when("#Commit", func() {
			it("should clear the staging dir", func() {
				layerTarPath := filepath.Join(stagingDir, "some-layer.tar")
				h.AssertNil(t, ioutil.WriteFile(layerTarPath, []byte("some data"), 0666))

				err := subject.Commit()
				h.AssertNil(t, err)

				_, err = os.Stat(layerTarPath)
				if err == nil {
					t.Fatal("expected staging dir to have been cleared")
				}
			})

			when("with #SetMetadata", func() {
				var newMetadata lifecycle.CacheMetadata

				it.Before(func() {
					previousContents := []byte(`{"buildpacks": [{"key": "old.bp.id"}]}`)
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), previousContents, 0666))

					newMetadata = lifecycle.CacheMetadata{
						Buildpacks: []lifecycle.BuildpackMetadata{{
							ID: "new.bp.id",
						}},
					}
				})

				when("set then commit", func() {
					it("retrieve returns the newly set metadata", func() {
						h.AssertNil(t, subject.SetMetadata(newMetadata))

						err := subject.Commit()
						h.AssertNil(t, err)

						retrievedMetadata, found, err := subject.RetrieveMetadata()
						h.AssertNil(t, err)
						h.AssertEq(t, found, true)
						h.AssertEq(t, retrievedMetadata, newMetadata)
					})
				})

				when("set without commit", func() {
					it("retrieve returns the previous metadata", func() {
						previousMetadata := lifecycle.CacheMetadata{
							Buildpacks: []lifecycle.BuildpackMetadata{{
								ID: "old.bp.id",
							}},
						}

						h.AssertNil(t, subject.SetMetadata(newMetadata))

						retrievedMetadata, found, err := subject.RetrieveMetadata()
						h.AssertNil(t, err)
						h.AssertEq(t, found, true)
						h.AssertEq(t, retrievedMetadata, previousMetadata)
					})
				})
			})

			when("with #AddLayer", func() {
				var tarPath string

				it.Before(func() {
					tarPath = filepath.Join(tmpDir, "some-layer.tar")
					h.AssertNil(t, ioutil.WriteFile(tarPath, []byte("dummy data"), 0666))
				})

				when("add then commit", func() {
					it("retrieve returns newly added layer", func() {
						h.AssertNil(t, subject.AddLayer("some_identifier", "some_sha", tarPath))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer("some_sha")
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

				when("add without commit", func() {
					it("retrieve returns not found error", func() {
						h.AssertNil(t, subject.AddLayer("some_identifier", "some_sha", tarPath))

						_, err := subject.RetrieveLayer("some_sha")
						h.AssertError(t, err, "layer with SHA 'some_sha' not found")
					})
				})

			})

			when("with #ReuseLayer", func() {
				it.Before(func() {
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(committedDir, "some_sha.tar"), []byte("dummy data"), 0666))
				})

				when("reuse then commit", func() {
					it("retrieve returns the reused layer", func() {
						h.AssertNil(t, subject.ReuseLayer("some_identifier", "some_sha"))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer("some_sha")
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

				when("reuse without commit", func() {
					it("retrieve returns the previous layer", func() {
						h.AssertNil(t, subject.ReuseLayer("some_identifier", "some_sha"))

						rc, err := subject.RetrieveLayer("some_sha")
						h.AssertNil(t, err)

						bytes, err := ioutil.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

			})
		})
	})
}
