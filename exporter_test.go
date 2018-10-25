package lifecycle_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/user"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/img"
)

func TestExporter(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Exporter", testExporter, spec.Report(report.Terminal{}))
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	var (
		exporter       *lifecycle.Exporter
		stdout, stderr *bytes.Buffer
		tmpDir         string
	)

	it.Before(func() {
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle.exporter.layer")
		if err != nil {
			t.Fatal(err)
		}
		exporter = &lifecycle.Exporter{
			TmpDir: tmpDir,
			Buildpacks: []*lifecycle.Buildpack{
				{ID: "buildpack.id"},
			},
			Out: io.MultiWriter(stdout, it.Out()),
			Err: io.MultiWriter(stderr, it.Out()),
		}
	})

	it.After(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatal(err)
		}
	})

	when("#Export", func() {
		var runImage v1.Image

		it.Before(func() {
			var err error
			runImage, err = getBusyboxWithEntrypoint()
			if err != nil {
				t.Fatalf("get busybox image for run image: %s", err)
			}
		})

		it("should process a simple launch directory", func() {
			image, err := exporter.Export("testdata/exporter/first/launch", "/launch/dest", runImage, nil)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			data, err := getMetadata(image)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}

			t.Log("adds buildpack metadata to label")
			if diff := cmp.Diff(data.Buildpacks[0].Key, "buildpack.id"); diff != "" {
				t.Fatal(diff)
			}

			t.Log("sets run image top layer in metadata")
			runImageLayerDigest, err := topLayer(runImage)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if diff := cmp.Diff(data.RunImage.TopLayer, runImageLayerDigest.String()); diff != "" {
				t.Fatalf(`RunImage digest did not match: (-got +want)\n%s`, diff)
			}

			t.Log("sets run image SHA in metadata")
			runImageDigest, err := runImage.Digest()
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if diff := cmp.Diff(data.RunImage.SHA, runImageDigest.String()); diff != "" {
				t.Fatalf(`RunImage digest did not match: (-got +want)\n%s`, diff)
			}

			t.Log("sets toml files in buildpack metadata")
			if diff := cmp.Diff(data.Buildpacks[0].Layers["layer1"].Data, map[string]interface{}{"mykey": "myval"}); diff != "" {
				t.Fatalf(`Layer toml did not match: (-got +want)\n%s`, diff)
			}

			t.Log("adds app layer to image")
			if txt, err := getImageFile(image, data.App.SHA, "/launch/dest/app/subdir/myfile.txt"); err != nil {
				t.Fatalf("Error: %s\n", err)
			} else if diff := cmp.Diff(strings.TrimSpace(txt), "mycontents"); diff != "" {
				t.Fatalf(`/launch/dest/app/subdir/myfile.txt: (-got +want)\n%s`, diff)
			}

			t.Log("adds config layer to image")
			if txt, err := getImageFile(image, data.Config.SHA, "/launch/dest/config/metadata.toml"); err != nil {
				t.Fatalf("Error: %s\n", err)
			} else if diff := cmp.Diff(strings.TrimSpace(txt), "[[processes]]\n  type = \"web\"\n  command = \"npm start\""); diff != "" {
				t.Fatalf(`/launch/dest/config/metadata.toml: (-got +want)\n%s`, diff)
			}

			t.Log("adds buildpack/layer1 as layer")
			if txt, err := getImageFile(image, data.Buildpacks[0].Layers["layer1"].SHA, "/launch/dest/buildpack.id/layer1/file-from-layer-1"); err != nil {
				t.Fatalf("Error: %s\n", err)
			} else if diff := cmp.Diff(strings.TrimSpace(txt), "echo text from layer 1"); diff != "" {
				t.Fatal("/launch/dest/buildpack.id/layer1/file-from-layer-1: (-got +want)", diff)
			}

			t.Log("adds buildpack/layer2 as layer")
			if txt, err := getImageFile(image, data.Buildpacks[0].Layers["layer2"].SHA, "/launch/dest/buildpack.id/layer2/file-from-layer-2"); err != nil {
				t.Fatalf("Error: %s\n", err)
			} else if diff := cmp.Diff(strings.TrimSpace(txt), "echo text from layer 2"); diff != "" {
				t.Fatal("/launch/dest/buildpack.id/layer2/file-from-layer-2: (-got +want)", diff)
			}

			t.Log("adds PACKS_LAUNCH_DIR env var")
			if val, err := envVar(image, "PACK_LAUNCH_DIR"); err != nil {
				t.Fatal(err)
			} else if val != "/launch/dest" {
				t.Fatalf("expected exporter to set env var PACK_LAUNCH_DIR to 'testdata/exporter/first/launch', got '%s'", val)
			}

			t.Log("uses filesystem uid/gid for image layers")
			currentUser, err := user.Current()
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if uid, gid, err := getImageFileOwner(image, data.App.SHA, "/launch/dest/app/subdir/myfile.txt"); err != nil {
				t.Fatalf("Error: %s\n", err)
			} else if diff := cmp.Diff(strconv.Itoa(uid), currentUser.Uid); diff != "" {
				t.Fatalf(`/launch/dest/app/subdir/myfile.txt: (-got +want)\n%s`, diff)
			} else if diff := cmp.Diff(strconv.Itoa(gid), currentUser.Gid); diff != "" {
				t.Fatalf(`/launch/dest/app/subdir/myfile.txt: (-got +want)\n%s`, diff)
			}
		})

		when("exporter has UID/GID set", func() {
			it.Before(func() {
				exporter.UID = 1234
				exporter.GID = 5678
			})
			it("sets uid/gid on the layer files", func() {
				image, err := exporter.Export("testdata/exporter/first/launch", "/launch/dest", runImage, nil)
				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				data, err := getMetadata(image)
				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}

				// File
				if uid, gid, err := getImageFileOwner(image, data.App.SHA, "/launch/dest/app/subdir/myfile.txt"); err != nil {
					t.Fatalf("Error: %s\n", err)
				} else if diff := cmp.Diff(uid, 1234); diff != "" {
					t.Fatalf(`/launch/dest/app/subdir/myfile.txt: (-got +want)\n%s`, diff)
				} else if diff := cmp.Diff(gid, 5678); diff != "" {
					t.Fatalf(`/launch/dest/app/subdir/myfile.txt: (-got +want)\n%s`, diff)
				}

				// Directory
				if uid, gid, err := getImageFileOwner(image, data.App.SHA, "/launch/dest/app/subdir"); err != nil {
					t.Fatalf("Error: %s\n", err)
				} else if diff := cmp.Diff(uid, 1234); diff != "" {
					t.Fatalf(`/launch/dest/app/subdir: (-got +want)\n%s`, diff)
				} else if diff := cmp.Diff(gid, 5678); diff != "" {
					t.Fatalf(`/launch/dest/app/subdir: (-got +want)\n%s`, diff)
				}
			})
		})

		when("rebuilding when layer TOML exists without directory", func() {
			var firstImage v1.Image
			it.Before(func() {
				var err error
				firstImage, err = exporter.Export("testdata/exporter/first/launch", "/launch/dest", runImage, nil)
				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}
			})

			it("should reuse layers if there is a layer TOML file", func() {
				image, err := exporter.Export("testdata/exporter/second/launch", "/launch/dest", runImage, firstImage)
				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				data, err := getMetadata(image)
				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}

				t.Log("sets toml files in image metadata")
				if diff := cmp.Diff(data.Buildpacks[0].Layers["layer1"].Data, map[string]interface{}{"mykey": "new val"}); diff != "" {
					t.Fatalf(`Layer toml did not match: (-got +want)\n%s`, diff)
				}

				t.Log("adds buildpack/layer1 as layer (from previous image)")
				if txt, err := getImageFile(image, data.Buildpacks[0].Layers["layer1"].SHA, "/launch/dest/buildpack.id/layer1/file-from-layer-1"); err != nil {
					t.Fatalf("Error: %s\n", err)
				} else if diff := cmp.Diff(strings.TrimSpace(txt), "echo text from layer 1"); diff != "" {
					t.Fatal("/launch/dest/buildpack.id/layer1/file-from-layer-1: (-got +want)", diff)
				}

				t.Log("adds buildpack/layer2 as layer from directory")
				if txt, err := getImageFile(image, data.Buildpacks[0].Layers["layer2"].SHA, "/launch/dest/buildpack.id/layer2/file-from-layer-2"); err != nil {
					t.Fatalf("Error: %s\n", err)
				} else if diff := cmp.Diff(strings.TrimSpace(txt), "echo text from new layer 2"); diff != "" {
					t.Fatal("/launch/dest/buildpack.id/layer2/file-from-layer-2: (-got +want)", diff)
				}

				t.Log("adds PACKS_LAUNCH_DIR env var")
				if val, err := envVar(image, "PACK_LAUNCH_DIR"); err != nil {
					t.Fatal(err)
				} else if val != "/launch/dest" {
					t.Fatalf("expected exporter to set env var PACK_LAUNCH_DIR to '/launch/dest', got '%s'", val)
				}
			})
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))
}

func topLayer(image v1.Image) (v1.Hash, error) {
	layers, err := image.Layers()
	if err != nil {
		return v1.Hash{}, err
	}
	return layers[len(layers)-1].DiffID()
}

func getBusyboxWithEntrypoint() (v1.Image, error) {
	runImageStore, err := img.NewRegistry("busybox")
	if err != nil {
		return nil, fmt.Errorf("get store for busybox: %s", err)
	}
	runImage, err := runImageStore.Image()
	if err != nil {
		return nil, fmt.Errorf("get image for busybox: %s", err)
	}
	configFile, err := runImage.ConfigFile()
	if err != nil {
		return nil, err
	}
	config := *configFile.Config.DeepCopy()
	config.Entrypoint = []string{"sh", "-c"}
	return mutate.Config(runImage, config)
}

func getLayerFile(layer v1.Layer, path string) (string, error) {
	r, err := layer.Uncompressed()
	if err != nil {
		return "", err
	}
	defer r.Close()
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err != nil {
			return "", err
		}

		if header.Name == path {
			buf, err := ioutil.ReadAll(tr)
			return string(buf), err
		}
	}
}

func getImageFile(image v1.Image, layerDigest, path string) (string, error) {
	hash, err := v1.NewHash(layerDigest)
	if err != nil {
		return "", err
	}
	layer, err := image.LayerByDiffID(hash)
	if err != nil {
		return "", err
	}
	return getLayerFile(layer, path)
}

func getImageFileOwner(image v1.Image, layerDigest, path string) (uid, gid int, err error) {
	hash, err := v1.NewHash(layerDigest)
	if err != nil {
		return 0, 0, err
	}
	layer, err := image.LayerByDiffID(hash)
	if err != nil {
		return 0, 0, err
	}

	r, err := layer.Uncompressed()
	if err != nil {
		return 0, 0, err
	}
	defer r.Close()
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err != nil {
			return 0, 0, err
		}

		if header.Name == path {
			return header.Uid, header.Gid, err
		}
	}
}

type metadata struct {
	RunImage struct {
		TopLayer string `json:"topLayer"`
		SHA      string `json:"sha"`
	} `json:"runImage"`
	App struct {
		SHA string `json:"sha"`
	} `json:"app"`
	Config struct {
		SHA string `json:"sha"`
	} `json:"config"`
	Buildpacks []struct {
		Key    string `json:"key"`
		Layers map[string]struct {
			SHA  string                 `json:"sha"`
			Data map[string]interface{} `json:"data"`
		} `json:"layers"`
	} `json:"buildpacks"`
}

func getMetadata(image v1.Image) (metadata, error) {
	var metadata metadata
	cfg, err := image.ConfigFile()
	if err != nil {
		return metadata, fmt.Errorf("read config: %s", err)
	}
	label := cfg.Config.Labels[lifecycle.MetadataLabel]
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return metadata, fmt.Errorf("unmarshal: %s", err)
	}
	return metadata, nil
}

func envVar(image v1.Image, key string) (string, error) {
	config, err := image.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("failed to get image config file: %s", err)
	}
	for _, e := range config.Config.Env {
		parts := strings.Split(e, "=")
		if parts[0] == key {
			return parts[1], nil
		}
	}
	return "", fmt.Errorf("image ENV did not contain variable '%s'", key)
}
