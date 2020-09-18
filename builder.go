package lifecycle

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/GoogleContainerTools/kaniko/pkg/util"
	"github.com/pkg/errors"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
)

type Builder struct {
	AppDir        string
	LayersDir     string
	PlatformDir   string
	BuildpacksDir string
	PlatformAPI   *api.Version
	Env           BuildEnv
	Group         BuildpackGroup
	Plan          BuildPlan
	Out, Err      *log.Logger
	Snapshotter   LayerSnapshotter
}

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string) error
	WithPlatform(platformDir string) ([]string, error)
	List() []string
}

type LaunchTOML struct {
	Labels    []Label
	Processes []launch.Process `toml:"processes"`
	Slices    []layers.Slice   `toml:"slices"`
}

type LayerSnapshotter interface {
	TakeSnapshot(string) error
	Init() error
}

type Label struct {
	Key   string `toml:"key"`
	Value string `toml:"value"`
}

type BOMEntry struct {
	Require
	Buildpack Buildpack `toml:"buildpack" json:"buildpack"`
}

type BuildpackPlan struct {
	Entries []Require `toml:"entries"`
}

func (b *Builder) Build() (*BuildMetadata, error) {
	layersDir, err := filepath.Abs(b.LayersDir)
	if err != nil {
		return nil, err
	}

	plan := b.Plan
	procMap := processMap{}
	var bom []BOMEntry
	var slices []layers.Slice
	var labels []Label

	useSnapshotter := b.Snapshotter != nil

	if useSnapshotter {
		if err := b.Snapshotter.Init(); err != nil {
			return nil, err
		}
	}

	for _, bp := range b.Group.Group {
		if useSnapshotter {
			// first, we apply the existing snapshots - if they exist from previous cache
			// this allows snapshot buildpacks to have cache and opt to do nothing
			err := bp.ApplyLayerSnapshots(b.LayersDir)
			if err != nil {
				return nil, err
			}
		}

		launchData, newPlan, bpBOM, err := b.build(bp, b.AppDir, plan)
		if err != nil {
			return nil, err
		}

		if useSnapshotter {
			bpDirName := launch.EscapeID(bp.ID)
			bpLayersDir := filepath.Join(layersDir, bpDirName)
			layerDir := filepath.Join(bpLayersDir, "snapshot")
			if err := os.MkdirAll(layerDir, 0777); err != nil {
				return nil, err
			}

			snapshotPath := filepath.Join(layerDir, "snapshot.tgz")
			if err := b.Snapshotter.TakeSnapshot(snapshotPath); err != nil {
				return nil, err
			}
			layerTOMLPath := filepath.Join(bpLayersDir, "snapshot.toml")
			md := BuildpackLayerMetadataFile{
				Cache:  true,
				Build:  true,
				Launch: false,
			}
			if err := WriteTOML(layerTOMLPath, md); err != nil {
				return nil, cmd.FailErr(err, "write build layer metadata")
			}
		}

		plan = newPlan
		bom = append(bom, bpBOM...)
		procMap.add(launchData.Processes)
		slices = append(slices, launchData.Slices...)
		labels = append(labels, launchData.Labels...)
	}

	if err := b.convertMetadataToVersion(bom); err != nil {
		return nil, err
	}

	return &BuildMetadata{
		BOM:        bom,
		Buildpacks: b.Group.Group,
		Labels:     labels,
		Processes:  procMap.list(),
		Slices:     slices,
	}, nil
}

func (bp Buildpack) ApplyLayerSnapshots(layersDir string) error {
	bpDir, err := readBuildpackLayersDir(layersDir, bp)
	if err != nil {
		return errors.Wrapf(err, "reading layers for buildpack '%s'", bp.ID)
	}

	for _, layer := range bpDir.findLayers(forCached) {
		layer := layer
		if !layer.hasLocalContents() {
			continue
		}
		_, err := layer.read()
		if err != nil {
			continue
		}

		snapshotFile := filepath.Join(layer.path, fmt.Sprintf("%s.tgz", layer.name()))
		_, err = util.UnpackLocalTarArchive(snapshotFile, "/")
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *Builder) build(bp Buildpack, rawAppDir string, plan BuildPlan) (LaunchTOML, BuildPlan, []BOMEntry, error) {
	platformDir, err := filepath.Abs(b.PlatformDir)
	if err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}
	layersDir, err := filepath.Abs(b.LayersDir)
	if err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}
	appDir, err := filepath.Abs(rawAppDir)
	if err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}
	defer os.RemoveAll(planDir)

	bpInfo, err := bp.Lookup(b.BuildpacksDir)
	if err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}

	bpDirName := launch.EscapeID(bp.ID)
	bpLayersDir := filepath.Join(layersDir, bpDirName)
	bpPlanDir := filepath.Join(planDir, bpDirName)
	if err := os.MkdirAll(bpLayersDir, 0777); err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}

	if err := os.MkdirAll(bpPlanDir, 0777); err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}
	bpPlanPath := filepath.Join(bpPlanDir, "plan.toml")

	foundPlan := plan.find(bp.noAPI())
	if api.MustParse(bp.API).Equal(api.MustParse("0.2")) {
		for i := range foundPlan.Entries {
			foundPlan.Entries[i].convertMetadataToVersion()
		}
	}
	if err := WriteTOML(bpPlanPath, foundPlan); err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}

	cmd := exec.Command(
		filepath.Join(bpInfo.Path, "bin", "build"),
		bpLayersDir,
		platformDir,
		bpPlanPath,
	)
	cmd.Dir = appDir
	cmd.Stdout = b.Out.Writer()
	cmd.Stderr = b.Err.Writer()

	if bpInfo.Buildpack.ClearEnv {
		cmd.Env = b.Env.List()
	} else {
		cmd.Env, err = b.Env.WithPlatform(platformDir)
		if err != nil {
			return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
		}
	}
	cmd.Env = append(cmd.Env, EnvBuildpackDir+"="+bpInfo.Path)

	if err := cmd.Run(); err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, NewLifecycleError(err, ErrTypeBuildpack)
	}
	if err := setupEnv(b.Env, bpLayersDir); err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}
	var bpPlanOut BuildpackPlan
	if _, err := toml.DecodeFile(bpPlanPath, &bpPlanOut); err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}
	var bpBOM []BOMEntry
	plan, bpBOM = plan.filter(bp, bpPlanOut)

	var launch LaunchTOML
	tomlPath := filepath.Join(bpLayersDir, "launch.toml")
	if _, err := toml.DecodeFile(tomlPath, &launch); os.IsNotExist(err) {
		return LaunchTOML{}, plan, bpBOM, nil
	} else if err != nil {
		return LaunchTOML{}, BuildPlan{}, []BOMEntry{}, err
	}
	for i := range launch.Processes {
		launch.Processes[i].BuildpackID = bp.ID
	}

	return launch, plan, bpBOM, nil
}

func (b Builder) convertMetadataToVersion(bom []BOMEntry) error {
	if b.PlatformAPI.Compare(api.MustParse("0.4")) < 0 {
		//plaformApiVersion is less than comparisonVersion
		for i := range bom {
			if err := bom[i].convertMetadataToVersion(); err != nil {
				return err
			}
		}
	} else {
		for i := range bom {
			if err := bom[i].convertVersionToMetadata(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p BuildPlan) find(bp Buildpack) BuildpackPlan {
	var out []Require
	for _, entry := range p.Entries {
		for _, provider := range entry.Providers {
			if provider == bp {
				out = append(out, entry.Requires...)
				break
			}
		}
	}
	return BuildpackPlan{Entries: out}
}

// TODO: ensure at least one claimed entry of each name is provided by the BP
func (p BuildPlan) filter(bp Buildpack, plan BuildpackPlan) (BuildPlan, []BOMEntry) {
	var out []BuildPlanEntry
	for _, entry := range p.Entries {
		if !plan.has(entry) {
			out = append(out, entry)
		}
	}
	var bom []BOMEntry
	for _, entry := range plan.Entries {
		bom = append(bom, BOMEntry{Require: entry, Buildpack: bp.noAPI()})
	}
	return BuildPlan{Entries: out}, bom
}

func (p BuildpackPlan) has(entry BuildPlanEntry) bool {
	for _, buildEntry := range p.Entries {
		for _, req := range entry.Requires {
			if req.Name == buildEntry.Name {
				return true
			}
		}
	}
	return false
}

func (bom *BOMEntry) convertMetadataToVersion() error {
	if version, ok := bom.Metadata["version"]; ok {
		metadataVersion := fmt.Sprintf("%v", version)
		if bom.Version != "" && bom.Version != metadataVersion {
			return errors.New("top level version does not match metadata version")
		}
		bom.Version = metadataVersion
	}
	return nil
}

func (bom *BOMEntry) convertVersionToMetadata() error {
	if bom.Version != "" {
		if bom.Metadata == nil {
			bom.Metadata = make(map[string]interface{})
		}
		if version, ok := bom.Metadata["version"]; ok {
			metadataVersion := fmt.Sprintf("%v", version)
			if metadataVersion != "" && metadataVersion != bom.Version {
				return errors.New("metadata version does not match top level version")
			}
		}
		bom.Metadata["version"] = bom.Version
		bom.Version = ""
	}
	return nil
}

func setupEnv(env BuildEnv, layersDir string) error {
	if err := eachDir(layersDir, func(path string) error {
		if !isBuild(path + ".toml") {
			return nil
		}
		return env.AddRootDir(path)
	}); err != nil {
		return err
	}

	return eachDir(layersDir, func(path string) error {
		if !isBuild(path + ".toml") {
			return nil
		}
		if err := env.AddEnvDir(filepath.Join(path, "env")); err != nil {
			return err
		}
		return env.AddEnvDir(filepath.Join(path, "env.build"))
	})
}

func eachDir(dir string, fn func(path string) error) error {
	files, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if err := fn(filepath.Join(dir, f.Name())); err != nil {
			return err
		}
	}
	return nil
}

func isBuild(path string) bool {
	var layerTOML struct {
		Build bool `toml:"build"`
	}
	_, err := toml.DecodeFile(path, &layerTOML)
	return err == nil && layerTOML.Build
}

type processMap map[string]launch.Process

func (m processMap) add(l []launch.Process) {
	for _, proc := range l {
		m[proc.Type] = proc
	}
}

func (m processMap) list() []launch.Process {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	procs := []launch.Process{}
	for _, key := range keys {
		procs = append(procs, m[key])
	}
	return procs
}
