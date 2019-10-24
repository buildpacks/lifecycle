package local

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"

	"github.com/buildpack/imgutil"
)

type Image struct {
	repoName         string
	docker           *client.Client
	inspect          types.ImageInspect
	layerPaths       []string
	currentTempImage string
	requestGroup     singleflight.Group
	prevName         string
	easyAddLayers    []string
}

type FileSystemLocalImage struct {
	dir       string
	layersMap map[string]string
}

type ImageOption func(image *Image) (*Image, error)

func WithPreviousImage(imageName string) ImageOption {
	return func(i *Image) (*Image, error) {
		if _, err := inspectOptionalImage(i.docker, imageName); err != nil {
			return i, err
		}

		i.prevName = imageName

		return i, nil
	}
}

func FromBaseImage(imageName string) ImageOption {
	return func(i *Image) (*Image, error) {
		var (
			err     error
			inspect types.ImageInspect
		)

		if inspect, err = inspectOptionalImage(i.docker, imageName); err != nil {
			return i, err
		}

		i.inspect = inspect
		i.layerPaths = make([]string, len(i.inspect.RootFS.Layers))

		return i, nil
	}
}

func NewImage(repoName string, dockerClient *client.Client, ops ...ImageOption) (imgutil.Image, error) {
	inspect := defaultInspect()

	image := &Image{
		docker:     dockerClient,
		repoName:   repoName,
		inspect:    inspect,
		layerPaths: make([]string, len(inspect.RootFS.Layers)),
	}

	var err error
	for _, v := range ops {
		image, err = v(image)
		if err != nil {
			return nil, err
		}
	}

	return image, nil
}

func (i *Image) Label(key string) (string, error) {
	labels := i.inspect.Config.Labels
	return labels[key], nil
}

func (i *Image) Env(key string) (string, error) {
	for _, envVar := range i.inspect.Config.Env {
		parts := strings.Split(envVar, "=")
		if parts[0] == key {
			return parts[1], nil
		}
	}
	return "", nil
}

func (i *Image) Rename(name string) {
	i.easyAddLayers = nil
	if prevInspect, _, err := i.docker.ImageInspectWithRaw(context.TODO(), name); err == nil {
		if i.sameBase(prevInspect) {
			i.easyAddLayers = prevInspect.RootFS.Layers[len(i.inspect.RootFS.Layers):]
		}
	}

	i.repoName = name
}

func (i *Image) sameBase(prevInspect types.ImageInspect) bool {
	if len(prevInspect.RootFS.Layers) < len(i.inspect.RootFS.Layers) {
		return false
	}
	for i, baseLayer := range i.inspect.RootFS.Layers {
		if baseLayer != prevInspect.RootFS.Layers[i] {
			return false
		}
	}
	return true
}

func (i *Image) Name() string {
	return i.repoName
}

func (i *Image) Found() bool {
	return i.inspect.ID != ""
}

func (i *Image) Identifier() (imgutil.Identifier, error) {
	return IDIdentifier{
		ImageID: strings.TrimPrefix(i.inspect.ID, "sha256:"),
	}, nil
}

func (i *Image) CreatedAt() (time.Time, error) {
	createdAtTime := i.inspect.Created
	createdTime, err := time.Parse(time.RFC3339Nano, createdAtTime)

	if err != nil {
		return time.Time{}, err
	}
	return createdTime, nil
}

func (i *Image) Rebase(baseTopLayer string, newBase imgutil.Image) error {
	ctx := context.Background()

	// FIND TOP LAYER
	keepLayers := -1
	for idx, diffID := range i.inspect.RootFS.Layers {
		if diffID == baseTopLayer {
			keepLayers = len(i.inspect.RootFS.Layers) - idx - 1
			break
		}
	}
	if keepLayers == -1 {
		return fmt.Errorf("'%s' not found in '%s' during rebase", baseTopLayer, i.repoName)
	}

	// SWITCH BASE LAYERS
	newBaseInspect, _, err := i.docker.ImageInspectWithRaw(ctx, newBase.Name())
	if err != nil {
		return errors.Wrap(err, "analyze read previous image config")
	}
	i.inspect.RootFS.Layers = newBaseInspect.RootFS.Layers
	i.layerPaths = make([]string, len(i.inspect.RootFS.Layers))

	// DOWNLOAD IMAGE
	fsImage, err := i.downloadImageOnce(i.repoName)
	if err != nil {
		return err
	}

	// READ MANIFEST.JSON
	b, err := ioutil.ReadFile(filepath.Join(fsImage.dir, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest []struct{ Layers []string }
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if len(manifest) != 1 {
		return fmt.Errorf("expected 1 image received %d", len(manifest))
	}

	// ADD EXISTING LAYERS
	for _, filename := range manifest[0].Layers[(len(manifest[0].Layers) - keepLayers):] {
		if err := i.AddLayer(filepath.Join(fsImage.dir, filename)); err != nil {
			return err
		}
	}

	return nil
}

func (i *Image) SetLabel(key, val string) error {
	if i.inspect.Config.Labels == nil {
		i.inspect.Config.Labels = map[string]string{}
	}

	i.inspect.Config.Labels[key] = val
	return nil
}

func (i *Image) SetEnv(key, val string) error {
	i.inspect.Config.Env = append(i.inspect.Config.Env, fmt.Sprintf("%s=%s", key, val))
	return nil
}

func (i *Image) SetWorkingDir(dir string) error {
	i.inspect.Config.WorkingDir = dir
	return nil
}

func (i *Image) SetEntrypoint(ep ...string) error {
	i.inspect.Config.Entrypoint = ep
	return nil
}

func (i *Image) SetCmd(cmd ...string) error {
	i.inspect.Config.Cmd = cmd
	return nil
}

func (i *Image) TopLayer() (string, error) {
	all := i.inspect.RootFS.Layers

	if len(all) == 0 {
		return "", fmt.Errorf("image '%s' has no layers", i.repoName)
	}

	topLayer := all[len(all)-1]
	return topLayer, nil
}

func (i *Image) GetLayer(diffID string) (io.ReadCloser, error) {
	fsImage, err := i.downloadImageOnce(i.repoName)
	if err != nil {
		return nil, err
	}

	layerID, ok := fsImage.layersMap[diffID]
	if !ok {
		return nil, fmt.Errorf("image '%s' does not contain layer with diff ID '%s'", i.repoName, diffID)
	}
	return os.Open(filepath.Join(fsImage.dir, layerID))
}

func (i *Image) AddLayer(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "AddLayer: open layer: %s", path)
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return errors.Wrapf(err, "AddLayer: calculate checksum: %s", path)
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	i.inspect.RootFS.Layers = append(i.inspect.RootFS.Layers, "sha256:"+sha)
	i.layerPaths = append(i.layerPaths, path)
	i.easyAddLayers = nil

	return nil
}

func (i *Image) ReuseLayer(diffID string) error {
	if len(i.easyAddLayers) > 0 && i.easyAddLayers[0] == diffID {
		i.inspect.RootFS.Layers = append(i.inspect.RootFS.Layers, diffID)
		i.layerPaths = append(i.layerPaths, "")
		i.easyAddLayers = i.easyAddLayers[1:]
		return nil
	}

	if i.prevName == "" {
		return errors.New("no previous image provided to reuse layers from")
	}

	fsImage, err := i.downloadImageOnce(i.prevName)
	if err != nil {
		return err
	}

	reuseLayer, ok := fsImage.layersMap[diffID]
	if !ok {
		return fmt.Errorf("SHA %s was not found in %s", diffID, i.repoName)
	}

	return i.AddLayer(filepath.Join(fsImage.dir, reuseLayer))
}

func (i *Image) Save(additionalNames ...string) error {
	inspect, err := i.doSave()
	if err != nil {
		saveErr := imgutil.SaveError{}
		for _, n := range append([]string{i.Name()}, additionalNames...) {
			saveErr.Errors = append(saveErr.Errors, imgutil.SaveDiagnostic{ImageName: n, Cause: err})
		}
		return saveErr
	}
	i.inspect = inspect

	var errs []imgutil.SaveDiagnostic
	for _, n := range append([]string{i.Name()}, additionalNames...) {
		if err := i.docker.ImageTag(context.Background(), i.inspect.ID, n); err != nil {
			errs = append(errs, imgutil.SaveDiagnostic{ImageName: n, Cause: err})
		}
	}

	if len(errs) > 0 {
		return imgutil.SaveError{Errors: errs}
	}

	return nil
}

func (i *Image) doSave() (types.ImageInspect, error) {
	ctx := context.Background()
	done := make(chan error)

	t, err := name.NewTag(i.repoName, name.WeakValidation)
	if err != nil {
		return types.ImageInspect{}, err
	}
	repoName := t.String()

	pr, pw := io.Pipe()
	defer pw.Close()
	go func() {
		res, err := i.docker.ImageLoad(ctx, pr, true)
		if err != nil {
			done <- err
			return
		}
		defer res.Body.Close()
		io.Copy(ioutil.Discard, res.Body)

		done <- nil
	}()

	tw := tar.NewWriter(pw)
	defer tw.Close()

	configFile, err := i.newConfigFile()
	if err != nil {
		return types.ImageInspect{}, errors.Wrap(err, "generate config file")
	}

	id := fmt.Sprintf("%x", sha256.Sum256(configFile))
	if err := addTextToTar(tw, id+".json", configFile); err != nil {
		return types.ImageInspect{}, err
	}

	var layerPaths []string
	for _, path := range i.layerPaths {
		if path == "" {
			layerPaths = append(layerPaths, "")
			continue
		}
		layerName := fmt.Sprintf("/%x.tar", sha256.Sum256([]byte(path)))
		f, err := os.Open(path)
		if err != nil {
			return types.ImageInspect{}, err
		}
		defer f.Close()
		if err := addFileToTar(tw, layerName, f); err != nil {
			return types.ImageInspect{}, err
		}
		f.Close()
		layerPaths = append(layerPaths, layerName)

	}

	manifest, err := json.Marshal([]map[string]interface{}{
		{
			"Config":   id + ".json",
			"RepoTags": []string{repoName},
			"Layers":   layerPaths,
		},
	})
	if err != nil {
		return types.ImageInspect{}, err
	}

	if err := addTextToTar(tw, "manifest.json", manifest); err != nil {
		return types.ImageInspect{}, err
	}

	tw.Close()
	pw.Close()
	err = <-done

	i.requestGroup.Forget(i.repoName)

	inspect, _, err := i.docker.ImageInspectWithRaw(context.Background(), id)
	if err != nil {
		if client.IsErrNotFound(err) {
			return types.ImageInspect{}, errors.Wrapf(err, "save image '%s'", i.repoName)
		}
		return types.ImageInspect{}, err
	}

	return inspect, nil
}

func (i *Image) newConfigFile() ([]byte, error) {
	imgConfig := map[string]interface{}{
		"os":      "linux",
		"created": time.Now().Format(time.RFC3339),
		"config":  i.inspect.Config,
		"rootfs": map[string][]string{
			"diff_ids": i.inspect.RootFS.Layers,
		},
		"history": make([]struct{}, len(i.inspect.RootFS.Layers)),
	}
	return json.Marshal(imgConfig)
}

func (i *Image) Delete() error {
	if !i.Found() {
		return nil
	}
	options := types.ImageRemoveOptions{
		Force:         true,
		PruneChildren: true,
	}
	_, err := i.docker.ImageRemove(context.Background(), i.inspect.ID, options)
	return err
}

func (i *Image) downloadImageOnce(imageName string) (*FileSystemLocalImage, error) {
	v, err, _ := i.requestGroup.Do(imageName, func() (details interface{}, err error) {
		return downloadImage(i.docker, imageName)
	})

	if err != nil {
		return nil, err
	}

	return v.(*FileSystemLocalImage), nil
}

func downloadImage(docker *client.Client, imageName string) (*FileSystemLocalImage, error) {
	ctx := context.Background()

	tarFile, err := docker.ImageSave(ctx, []string{imageName})
	if err != nil {
		return nil, err
	}
	defer tarFile.Close()

	tmpDir, err := ioutil.TempDir("", "imgutil.local.image.")
	if err != nil {
		return nil, errors.Wrap(err, "local reuse-layer create temp dir")
	}

	err = untar(tarFile, tmpDir)
	if err != nil {
		return nil, err
	}

	mf, err := os.Open(filepath.Join(tmpDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	defer mf.Close()

	var manifest []struct {
		Config string
		Layers []string
	}
	if err := json.NewDecoder(mf).Decode(&manifest); err != nil {
		return nil, err
	}

	if len(manifest) != 1 {
		return nil, fmt.Errorf("manifest.json had unexpected number of entries: %d", len(manifest))
	}

	df, err := os.Open(filepath.Join(tmpDir, manifest[0].Config))
	if err != nil {
		return nil, err
	}
	defer df.Close()

	var details struct {
		RootFS struct {
			DiffIDs []string `json:"diff_ids"`
		} `json:"rootfs"`
	}

	if err = json.NewDecoder(df).Decode(&details); err != nil {
		return nil, err
	}

	if len(manifest[0].Layers) != len(details.RootFS.DiffIDs) {
		return nil, fmt.Errorf("layers and diff IDs do not match, there are %d layers and %d diffIDs", len(manifest[0].Layers), len(details.RootFS.DiffIDs))
	}

	layersMap := make(map[string]string, len(manifest[0].Layers))
	for i, diffID := range details.RootFS.DiffIDs {
		layerID := manifest[0].Layers[i]
		layersMap[diffID] = layerID
	}

	return &FileSystemLocalImage{
		dir:       tmpDir,
		layersMap: layersMap,
	}, nil
}

func addTextToTar(tw *tar.Writer, name string, contents []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(contents))}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(contents)
	return err
}

func addFileToTar(tw *tar.Writer, name string, contents *os.File) error {
	fi, err := contents.Stat()
	if err != nil {
		return err
	}
	hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(fi.Size())}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, contents)
	return err
}

func untar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			return nil
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			_, err := os.Stat(filepath.Dir(path))
			if os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return err
				}
			}

			fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(fh, tr); err != nil {
				fh.Close()
				return err
			}
			fh.Close()
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown file type in tar %d", hdr.Typeflag)
		}
	}
}

func inspectOptionalImage(docker *client.Client, imageName string) (types.ImageInspect, error) {
	var (
		err     error
		inspect types.ImageInspect
	)

	if inspect, _, err = docker.ImageInspectWithRaw(context.Background(), imageName); err != nil {
		if client.IsErrNotFound(err) {
			return defaultInspect(), nil
		}

		return types.ImageInspect{}, errors.Wrapf(err, "verifying image '%s'", imageName)
	}

	return inspect, nil
}

func defaultInspect() types.ImageInspect {
	return types.ImageInspect{
		Config: &container.Config{},
	}
}
