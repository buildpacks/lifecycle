package cache_test

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/log"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestVolumeCache(t *testing.T) {
	spec.Run(t, "VolumeCache", testVolumeCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testVolumeCache(t *testing.T, when spec.G, it spec.S) {
	const (
		validDiffID       = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		nonExistentDiffID = "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	)

	var (
		tmpDir       string
		volumeDir    string
		subject      *cache.VolumeCache
		backupDir    string
		stagingDir   string
		committedDir string
		testLogger   log.Logger
	)

	it.Before(func() {
		var err error

		tmpDir, err = os.MkdirTemp("", "lifecycle.cache.volume_cache")
		h.AssertNil(t, err)

		volumeDir = filepath.Join(tmpDir, "test_volume")
		h.AssertNil(t, os.MkdirAll(volumeDir, os.ModePerm))

		backupDir = filepath.Join(volumeDir, "committed-backup")
		stagingDir = filepath.Join(volumeDir, "staging")
		committedDir = filepath.Join(volumeDir, "committed")
		testLogger = cmd.DefaultLogger
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#NewVolumeCache", func() {
		it("returns an error when the volume path does not exist", func() {
			_, err := cache.NewVolumeCache(filepath.Join(tmpDir, "does_not_exist"), testLogger)
			if err == nil {
				t.Fatal("expected NewVolumeCache to fail because volume path does not exist")
			}
		})

		when("staging already exists", func() {
			it.Before(func() {
				stagingPath := filepath.Join(volumeDir, "staging")
				h.AssertNil(t, os.MkdirAll(stagingPath, 0777))
				h.AssertNil(t, os.WriteFile(filepath.Join(stagingPath, "some-layer.tar"), []byte("some data"), 0600))
			})

			it("clears staging", func() {
				var err error

				subject, err = cache.NewVolumeCache(volumeDir, testLogger)
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

				subject, err = cache.NewVolumeCache(volumeDir, testLogger)
				h.AssertNil(t, err)

				_, err = os.Stat(stagingDir)
				h.AssertNil(t, err)
			})
		})

		when("committed does not exist", func() {
			it("creates committed dir", func() {
				var err error

				subject, err = cache.NewVolumeCache(volumeDir, testLogger)
				h.AssertNil(t, err)

				_, err = os.Stat(committedDir)
				h.AssertNil(t, err)
			})
		})

		when("backup dir already exists", func() {
			it.Before(func() {
				h.AssertNil(t, os.MkdirAll(backupDir, 0777))
				h.AssertNil(t, os.WriteFile(filepath.Join(backupDir, "some-layer.tar"), []byte("some data"), 0600))
			})

			it("clears the backup dir", func() {
				var err error

				subject, err = cache.NewVolumeCache(volumeDir, testLogger)
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

			subject, err = cache.NewVolumeCache(volumeDir, testLogger)
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
					h.AssertNil(t, os.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), content, 0600))
				})

				it("returns the metadata", func() {
					expected := platform.CacheMetadata{
						Buildpacks: []buildpack.LayersMetadata{{
							ID:      "bp.id",
							Version: "1.2.3",
							Layers: map[string]buildpack.LayerMetadata{
								"some-layer": {
									SHA: "some-sha",
									LayerMetadataFile: buildpack.LayerMetadataFile{
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
					h.AssertNil(t, os.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), []byte("garbage"), 0600))
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
					h.AssertNil(t, os.WriteFile(filepath.Join(committedDir, validDiffID+".tar"), []byte("dummy data"), 0600))
				})

				it("returns the layer's reader", func() {
					rc, err := subject.RetrieveLayer(validDiffID)
					h.AssertNil(t, err)

					bytes, err := io.ReadAll(rc)
					h.AssertNil(t, err)
					h.AssertEq(t, string(bytes), "dummy data")
				})
			})

			when("layer does not exist", func() {
				it("returns an error", func() {
					_, err := subject.RetrieveLayer(nonExistentDiffID)
					h.AssertError(t, err, fmt.Sprintf("failed to find cache layer with SHA '%s'", nonExistentDiffID))
				})
			})
		})

		when("#RetrieveLayerFile", func() {
			when("layer exists", func() {
				it.Before(func() {
					h.AssertNil(t, os.WriteFile(filepath.Join(committedDir, validDiffID+".tar"), []byte("dummy data"), 0600))
				})

				it("returns the layer's reader", func() {
					layerPath, err := subject.RetrieveLayerFile(validDiffID)
					h.AssertNil(t, err)

					bytes, err := os.ReadFile(layerPath)
					h.AssertNil(t, err)
					h.AssertEq(t, string(bytes), "dummy data")
				})
			})

			when("layer does not exist", func() {
				it("returns an error", func() {
					_, err := subject.RetrieveLayerFile(nonExistentDiffID)
					h.AssertError(t, err, fmt.Sprintf("failed to find cache layer with SHA '%s'", nonExistentDiffID))
				})
			})
		})

		when("#Commit", func() {
			it("should clear the staging dir", func() {
				layerTarPath := filepath.Join(stagingDir, "some-layer.tar")
				h.AssertNil(t, os.WriteFile(layerTarPath, []byte("some data"), 0600))

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
					h.AssertNil(t, os.WriteFile(filepath.Join(committedDir, "io.buildpacks.lifecycle.cache.metadata"), previousContents, 0600))

					newMetadata = platform.CacheMetadata{
						Buildpacks: []buildpack.LayersMetadata{{
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
							Buildpacks: []buildpack.LayersMetadata{{
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
					h.AssertNil(t, os.WriteFile(tarPath, []byte("dummy data"), 0600))
				})

				when("add then commit", func() {
					it("retrieve returns newly added layer", func() {
						h.AssertNil(t, subject.AddLayerFile(tarPath, validDiffID))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer(validDiffID)
						h.AssertNil(t, err)

						bytes, err := io.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

				when("add after commit", func() {
					it("retrieve returns the newly set metadata", func() {
						err := subject.Commit()
						h.AssertNil(t, err)

						h.AssertError(t, subject.AddLayerFile(tarPath, validDiffID), "cache cannot be modified after commit")
					})
				})

				when("add without commit", func() {
					it("retrieve returns not found error", func() {
						h.AssertNil(t, subject.AddLayerFile(tarPath, validDiffID))

						_, err := subject.RetrieveLayer(validDiffID)
						h.AssertError(t, err, fmt.Sprintf("failed to find cache layer with SHA '%s'", validDiffID))
					})
				})

				when("a layer with the same sha already exists", func() {
					it.Before(func() {
						existingLayerTar, err := os.CreateTemp("", "*.tar")
						h.AssertNil(t, err)
						h.AssertNil(t, os.WriteFile(existingLayerTar.Name(), []byte("existing data"), 0600))
						h.AssertNil(t, subject.AddLayerFile(existingLayerTar.Name(), validDiffID))
					})

					it("does nothing", func() {
						h.AssertNil(t, subject.AddLayerFile(tarPath, validDiffID))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer(validDiffID)
						h.AssertNil(t, err)

						bytes, err := io.ReadAll(rc)
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

						bytes, err := io.ReadAll(rc)
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
						h.AssertError(t, err, fmt.Sprintf("failed to find cache layer with SHA '%s'", layerSha))
					})
				})

				when("a layer with the same sha already exists", func() {
					it.Before(func() {
						existingLayerTar, err := os.CreateTemp("", "*.tar")
						h.AssertNil(t, err)
						h.AssertNil(t, os.WriteFile(existingLayerTar.Name(), layerData, 0600))
						h.AssertNil(t, subject.AddLayerFile(existingLayerTar.Name(), layerSha))
					})

					it("succeeds", func() {
						h.AssertNil(t, subject.AddLayer(layerReader, layerSha))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer(layerSha)
						h.AssertNil(t, err)

						bytes, err := io.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, bytes, layerData)
					})
				})
			})

			when("#ReuseLayer", func() {
				it.Before(func() {
					h.AssertNil(t, os.WriteFile(filepath.Join(committedDir, validDiffID+".tar"), []byte("dummy data"), 0600))
				})

				when("reuse then commit", func() {
					it("retrieve returns the reused layer", func() {
						h.AssertNil(t, subject.ReuseLayer(validDiffID))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer(validDiffID)
						h.AssertNil(t, err)

						bytes, err := io.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

				when("reuse after commit", func() {
					it("retrieve returns the newly set metadata", func() {
						err := subject.Commit()
						h.AssertNil(t, err)

						h.AssertError(t, subject.ReuseLayer(validDiffID), "cache cannot be modified after commit")
					})
				})

				when("reuse without commit", func() {
					it("retrieve returns the previous layer", func() {
						h.AssertNil(t, subject.ReuseLayer(validDiffID))

						rc, err := subject.RetrieveLayer(validDiffID)
						h.AssertNil(t, err)

						bytes, err := io.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "dummy data")
					})
				})

				when("a layer with the same sha already exists", func() {
					it.Before(func() {
						tarPath := filepath.Join(tmpDir, "some-layer.tar")
						h.AssertNil(t, os.WriteFile(tarPath, []byte("existing data"), 0600))
						h.AssertNil(t, subject.AddLayerFile(tarPath, validDiffID))
					})

					it("does nothing", func() {
						h.AssertNil(t, subject.ReuseLayer(validDiffID))

						err := subject.Commit()
						h.AssertNil(t, err)

						rc, err := subject.RetrieveLayer(validDiffID)
						h.AssertNil(t, err)

						bytes, err := io.ReadAll(rc)
						h.AssertNil(t, err)
						h.AssertEq(t, string(bytes), "existing data")
					})
				})

				when("the layer does not exist", func() {
					it("fails with a read error", func() {
						err := subject.ReuseLayer(nonExistentDiffID)
						isReadErr, _ := cache.IsReadErr(err)
						h.AssertEq(t, isReadErr, true)

						err = subject.Commit()
						h.AssertNil(t, err)

						_, err = subject.RetrieveLayer(nonExistentDiffID)
						isReadErr, _ = cache.IsReadErr(err)
						h.AssertEq(t, isReadErr, true)
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

		when("validateDiffID", func() {
			testCases := []struct {
				name        string
				diffID      string
				expectError bool
			}{
				{
					name:        "valid canonical diffID",
					diffID:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					expectError: false,
				},
				{
					name:        "valid real sha256 diffID",
					diffID:      "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					expectError: false,
				},
				{
					name:        "empty string",
					diffID:      "",
					expectError: true,
				},
				{
					name:        "00",
					diffID:      "00",
					expectError: true,
				},
				{
					name:        "+0",
					diffID:      "+0",
					expectError: true,
				},
				{
					name:        "0x0",
					diffID:      "0x0",
					expectError: true,
				},
				{
					name:        " 0",
					diffID:      " 0",
					expectError: true,
				},
				{
					name:        "0 ",
					diffID:      "0 ",
					expectError: true,
				},
				{
					name:        "root ",
					diffID:      "root ",
					expectError: true,
				},
				{
					name:        "unicode digits",
					diffID:      "sha256:٠١٢٣٤٥٦٧٨٩abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					expectError: true,
				},
				{
					name:        "missing sha256 prefix",
					diffID:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					expectError: true,
				},
				{
					name:        "wrong algorithm prefix",
					diffID:      "sha512:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					expectError: true,
				},
				{
					name:        "uppercase hex",
					diffID:      "sha256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef",
					expectError: true,
				},
				{
					name:        "mixed case hex",
					diffID:      "sha256:0123456789Abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					expectError: true,
				},
				{
					name:        "short hex",
					diffID:      "sha256:abcd",
					expectError: true,
				},
				{
					name:        "long hex",
					diffID:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0",
					expectError: true,
				},
				{
					name:        "non-hex characters",
					diffID:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdeg",
					expectError: true,
				},
				{
					name:        "path traversal without prefix",
					diffID:      "../../etc/passwd",
					expectError: true,
				},
				{
					name:        "path traversal with prefix",
					diffID:      "sha256:../../etc/passwd",
					expectError: true,
				},
				{
					name:        "path traversal parent directory",
					diffID:      "sha256:../foo",
					expectError: true,
				},
				{
					name:        "path traversal slash",
					diffID:      "sha256:foo/bar",
					expectError: true,
				},
				{
					name:        "leading whitespace",
					diffID:      " sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					expectError: true,
				},
				{
					name:        "trailing whitespace",
					diffID:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef ",
					expectError: true,
				},
				{
					name:        "trailing null byte",
					diffID:      "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\x00",
					expectError: true,
				},
			}

			for _, tc := range testCases {
				it(fmt.Sprintf("handles %s", tc.name), func() {
					err := cache.ValidateDiffID(tc.diffID)
					if tc.expectError {
						h.AssertError(t, err, fmt.Sprintf("invalid diffID %q: must be sha256:<64 lowercase hex>", tc.diffID))
					} else {
						h.AssertNil(t, err)
					}
				})
			}
		})

		when("methods handle malformed diffID per spec", func() {
			const malformedDiffID = "sha256:invalid-or-traversal/../"

			when("#AddLayerFile", func() {
				it("returns a hard error on malformed diffID", func() {
					tarPath := filepath.Join(tmpDir, "some-layer.tar")
					h.AssertNil(t, os.WriteFile(tarPath, []byte("dummy data"), 0600))

					err := subject.AddLayerFile(tarPath, malformedDiffID)
					h.AssertError(t, err, fmt.Sprintf("invalid diffID %q: must be sha256:<64 lowercase hex>", malformedDiffID))
					isReadErr, _ := cache.IsReadErr(err)
					h.AssertEq(t, isReadErr, false)
				})
			})

			when("#AddLayer", func() {
				it("returns a hard error on malformed diffID", func() {
					rc := io.NopCloser(strings.NewReader("dummy data"))
					err := subject.AddLayer(rc, malformedDiffID)
					h.AssertError(t, err, fmt.Sprintf("invalid diffID %q: must be sha256:<64 lowercase hex>", malformedDiffID))
					isReadErr, _ := cache.IsReadErr(err)
					h.AssertEq(t, isReadErr, false)
				})
			})

			when("#ReuseLayer", func() {
				it("returns a hard error on malformed diffID", func() {
					err := subject.ReuseLayer(malformedDiffID)
					h.AssertError(t, err, fmt.Sprintf("invalid diffID %q: must be sha256:<64 lowercase hex>", malformedDiffID))
					isReadErr, _ := cache.IsReadErr(err)
					h.AssertEq(t, isReadErr, false)
				})
			})

			when("#HasLayer", func() {
				it("returns (false, nil) pure miss on malformed diffID", func() {
					hasLayer, err := subject.HasLayer(malformedDiffID)
					h.AssertNil(t, err)
					h.AssertEq(t, hasLayer, false)
				})

				it("returns true for existing valid layer and false for non-existent valid layer", func() {
					h.AssertNil(t, os.WriteFile(filepath.Join(committedDir, validDiffID+".tar"), []byte("data"), 0600))
					hasLayer, err := subject.HasLayer(validDiffID)
					h.AssertNil(t, err)
					h.AssertEq(t, hasLayer, true)

					hasLayer, err = subject.HasLayer(nonExistentDiffID)
					h.AssertNil(t, err)
					h.AssertEq(t, hasLayer, false)
				})
			})

			when("#RetrieveLayer", func() {
				it("returns a ReadErr on malformed diffID", func() {
					rc, err := subject.RetrieveLayer(malformedDiffID)
					h.AssertNil(t, rc)
					isReadErr, readErr := cache.IsReadErr(err)
					h.AssertEq(t, isReadErr, true)
					h.AssertEq(t, readErr.Error(), fmt.Sprintf("invalid diffID %q: must be sha256:<64 lowercase hex>", malformedDiffID))
				})
			})

			when("#RetrieveLayerFile", func() {
				it("returns a ReadErr on malformed diffID", func() {
					path, err := subject.RetrieveLayerFile(malformedDiffID)
					h.AssertEq(t, path, "")
					isReadErr, readErr := cache.IsReadErr(err)
					h.AssertEq(t, isReadErr, true)
					h.AssertEq(t, readErr.Error(), fmt.Sprintf("invalid diffID %q: must be sha256:<64 lowercase hex>", malformedDiffID))
				})
			})

			when("#VerifyLayer", func() {
				it("returns a ReadErr on malformed diffID", func() {
					err := subject.VerifyLayer(malformedDiffID)
					isReadErr, readErr := cache.IsReadErr(err)
					h.AssertEq(t, isReadErr, true)
					h.AssertEq(t, readErr.Error(), fmt.Sprintf("invalid diffID %q: must be sha256:<64 lowercase hex>", malformedDiffID))
				})

				it("returns nil when layer hash matches diffID and ReadErr when it mismatches", func() {
					tarPath := filepath.Join(tmpDir, "layer.tar")
					data := []byte("some layer data")
					h.AssertNil(t, os.WriteFile(tarPath, data, 0600))
					hasher := sha256.New()
					hasher.Write(data)
					matchingSHA := fmt.Sprintf("sha256:%x", hasher.Sum(nil))

					h.AssertNil(t, subject.AddLayerFile(tarPath, matchingSHA))
					h.AssertNil(t, subject.Commit())

					h.AssertNil(t, subject.VerifyLayer(matchingSHA))

					otherSHA := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
					h.AssertNil(t, os.WriteFile(filepath.Join(committedDir, otherSHA+".tar"), data, 0600))
					err := subject.VerifyLayer(otherSHA)
					isReadErr, _ := cache.IsReadErr(err)
					h.AssertEq(t, isReadErr, true)
				})
			})
		})
	})
}
