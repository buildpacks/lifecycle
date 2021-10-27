package layer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/buildpack/layermetadata"
	"github.com/buildpacks/lifecycle/launch"
)

type BpLayersDir struct {
	Path      string
	layers    []BpLayer
	name      string
	Buildpack buildpack.GroupBuildpack
	Store     *buildpack.StoreTOML
}

func ReadBuildpackLayersDir(layersDir string, bp buildpack.GroupBuildpack, logger Logger) (BpLayersDir, error) {
	path := filepath.Join(layersDir, launch.EscapeID(bp.ID))
	logger.Debugf("Reading buildpack directory: %s", path)
	bpDir := BpLayersDir{
		name:      bp.ID,
		Path:      path,
		layers:    []BpLayer{},
		Buildpack: bp,
	}

	fis, err := ioutil.ReadDir(path)
	if err != nil && !os.IsNotExist(err) {
		return BpLayersDir{}, err
	}

	names := map[string]struct{}{}
	var tomls []string
	for _, fi := range fis {
		logger.Debugf("Reading buildpack directory item: %s", fi.Name())
		if fi.IsDir() {
			bpDir.layers = append(bpDir.layers, *bpDir.NewBPLayer(fi.Name(), bp.API, logger))
			names[fi.Name()] = struct{}{}
			continue
		}
		if strings.HasSuffix(fi.Name(), ".toml") {
			tomls = append(tomls, filepath.Join(path, fi.Name()))
		}
	}

	for _, tf := range tomls {
		name := strings.TrimSuffix(filepath.Base(tf), ".toml")
		if name == "store" {
			var bpStore buildpack.StoreTOML
			_, err := toml.DecodeFile(tf, &bpStore)
			if err != nil {
				return BpLayersDir{}, errors.Wrapf(err, "failed decoding store.toml for buildpack %q", bp.ID)
			}
			bpDir.Store = &bpStore
			continue
		}
		if name == "launch" {
			// don't treat launch.toml as a layer
			continue
		}
		if name == "build" && api.MustParse(bp.API).AtLeast("0.5") {
			// if the buildpack API supports build.toml don't treat it as a layer
			continue
		}
		if _, ok := names[name]; !ok {
			bpDir.layers = append(bpDir.layers, *bpDir.NewBPLayer(name, bp.API, logger))
		}
	}
	sort.Slice(bpDir.layers, func(i, j int) bool {
		return bpDir.layers[i].identifier < bpDir.layers[j].identifier
	})
	return bpDir, nil
}

func ForLaunch(l BpLayer) bool {
	md, err := l.Read()
	return err == nil && md.Launch
}

func ForMalformed(l BpLayer) bool {
	_, err := l.Read()
	return err != nil
}

func ForCached(l BpLayer) bool {
	md, err := l.Read()
	return err == nil && md.Cache
}

func (bd *BpLayersDir) FindLayers(f func(layer BpLayer) bool) []BpLayer {
	var selectedLayers []BpLayer
	for _, l := range bd.layers {
		if f(l) {
			selectedLayers = append(selectedLayers, l)
		}
	}
	return selectedLayers
}

func (bd *BpLayersDir) NewBPLayer(name, buildpackAPI string, logger Logger) *BpLayer {
	return &BpLayer{
		layer: layer{
			path:       filepath.Join(bd.Path, name),
			identifier: fmt.Sprintf("%s:%s", bd.Buildpack.ID, name),
		},
		api:    buildpackAPI,
		logger: logger,
	}
}

type BpLayer struct { // TODO: need to refactor so api and logger won't be part of this struct
	layer
	api    string
	logger Logger
}

func (bp *BpLayer) Read() (buildpack.LayerMetadata, error) {
	tomlPath := bp.Path() + ".toml"
	layerMetadataFile, msg, err := buildpack.DecodeLayerMetadataFile(tomlPath, bp.api)
	if err != nil {
		return buildpack.LayerMetadata{}, err
	}
	if msg != "" {
		if api.MustParse(bp.api).LessThan("0.6") {
			bp.logger.Warn(msg)
		} else {
			return buildpack.LayerMetadata{}, errors.New(msg)
		}
	}
	var sha string
	shaBytes, err := ioutil.ReadFile(bp.Path() + ".sha")
	if err != nil && !os.IsNotExist(err) { // if the sha file doesn't exist, an empty sha will be returned
		return buildpack.LayerMetadata{}, err
	}
	if err == nil {
		sha = string(shaBytes)
	}
	return buildpack.LayerMetadata{SHA: sha, File: layerMetadataFile}, nil
}

func (bp *BpLayer) Remove() error {
	if err := os.RemoveAll(bp.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(bp.path + ".sha"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(bp.path + ".toml"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (bp *BpLayer) WriteMetadata(metadata layermetadata.File) error {
	path := filepath.Join(bp.path + ".toml")
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	return buildpack.EncodeLayerMetadataFile(metadata, path, bp.api)
}

func (bp *BpLayer) HasLocalContents() bool {
	_, err := ioutil.ReadDir(bp.path)

	return !os.IsNotExist(err)
}

func (bp *BpLayer) WriteSha(sha string) error {
	if err := ioutil.WriteFile(bp.path+".sha", []byte(sha), 0666); err != nil {
		return err
	} // #nosec G306
	return nil
}

func (bp *BpLayer) Name() string {
	return filepath.Base(bp.path)
}

type layer struct {
	path       string
	identifier string
}

func (l *layer) Identifier() string {
	return l.identifier
}

func (l *layer) Path() string {
	return l.path
}

type Dir interface {
	Identifier() string
	Path() string
}
