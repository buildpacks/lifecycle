package lifecycle

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
)

type BuildEnv interface {
	AddRootDir(baseDir string) error
	AddEnvDir(envDir string, defaultAction env.ActionType) error
	WithPlatform(platformDir string) ([]string, error)
	List() []string
}

type BuildpackFinder interface {
	Find(bpID, bpVersion, buildpacksDir string) (BuildpackTOML, error)
}

type DefaultBuildpackFinder struct{}

// TODO: this duplicates code in buildpack.go
func (f *DefaultBuildpackFinder) Find(bpID, bpVersion, buildpacksDir string) (BuildpackTOML, error) {
	bpTOML := DefaultBuildpackTOML{}
	bpPath, err := filepath.Abs(filepath.Join(buildpacksDir, launch.EscapeID(bpID), bpVersion))
	if err != nil {
		return nil, err
	}
	tomlPath := filepath.Join(bpPath, "buildpack.toml")
	if _, err := toml.DecodeFile(tomlPath, &bpTOML); err != nil {
		return nil, err
	}
	bpTOML.Path = bpPath
	return &bpTOML, nil
}

type BuildpackTOML interface {
	Build(bpPlan BuildpackPlan, config BuildConfig) (BuildResult, error)
}

type BuildConfig struct {
	Env         BuildEnv
	AppDir      string
	PlatformDir string
	LayersDir   string
	PlanDir     string
	Out         io.Writer
	Err         io.Writer
}

type BuildResult struct {
	BOM       []BOMEntry
	Labels    []Label
	Met       []string
	Processes []launch.Process
	Slices    []layers.Slice
}

type BOMEntry struct {
	Require
	Buildpack Buildpack `toml:"buildpack" json:"buildpack"`
}

type Label struct {
	Key   string `toml:"key"`
	Value string `toml:"value"`
}

type BuildpackPlan struct {
	Entries []Require `toml:"entries"`
}

type Builder struct {
	AppDir          string
	LayersDir       string
	PlatformDir     string
	BuildpacksDir   string
	PlatformAPI     *api.Version
	Env             BuildEnv
	Group           BuildpackGroup
	Plan            BuildPlan
	Out, Err        io.Writer
	BuildpackFinder BuildpackFinder
	planDir         string
}

func (b *Builder) Build() (*BuildMetadata, error) {
	config, err := b.BuildConfig()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(config.PlanDir)

	procMap := processMap{}
	plan := b.Plan
	var bom []BOMEntry
	var slices []layers.Slice
	var labels []Label

	for _, bp := range b.Group.Group {
		bpTOML, err := b.BuildpackFinder.Find(bp.ID, bp.Version, b.BuildpacksDir)
		if err != nil {
			return nil, err
		}

		bpPlan := plan.find(bp.ID)
		br, err := bpTOML.Build(bpPlan, config)
		if err != nil {
			return nil, err
		}

		bom = append(bom, br.BOM...)
		labels = append(labels, br.Labels...)
		plan = plan.filter(br.Met)
		procMap.add(br.Processes)
		slices = append(slices, br.Slices...)
	}

	if b.PlatformAPI.Compare(api.MustParse("0.4")) < 0 { // PlatformAPI <= 0.3
		for i := range bom {
			bom[i].convertMetadataToVersion()
		}
	} else {
		for i := range bom {
			bom[i].convertVersionToMetadata()
		}
	}

	return &BuildMetadata{
		BOM:        bom,
		Buildpacks: b.Group.Group,
		Labels:     labels,
		Processes:  procMap.list(),
		Slices:     slices,
	}, nil
}

func (b *Builder) BuildConfig() (BuildConfig, error) {
	appDir, err := filepath.Abs(b.AppDir)
	if err != nil {
		return BuildConfig{}, err
	}
	platformDir, err := filepath.Abs(b.PlatformDir)
	if err != nil {
		return BuildConfig{}, err
	}
	layersDir, err := filepath.Abs(b.LayersDir)
	if err != nil {
		return BuildConfig{}, err
	}
	if b.planDir == "" {
		planDir, err := ioutil.TempDir("", "plan.")
		if err != nil {
			return BuildConfig{}, err
		}
		b.planDir = planDir
	}

	return BuildConfig{
		Env:         b.Env,
		AppDir:      appDir,
		PlatformDir: platformDir,
		LayersDir:   layersDir,
		PlanDir:     b.planDir,
		Out:         b.Out,
		Err:         b.Err,
	}, nil
}

func (p BuildPlan) find(bpID string) BuildpackPlan {
	var out []Require
	for _, entry := range p.Entries {
		for _, provider := range entry.Providers {
			if provider.ID == bpID {
				out = append(out, entry.Requires...)
				break
			}
		}
	}
	return BuildpackPlan{Entries: out}
}

// TODO: ensure at least one claimed entry of each name is provided by the BP
func (p BuildPlan) filter(metRequires []string) BuildPlan {
	var out []BuildPlanEntry
	for _, planEntry := range p.Entries {
		if !containsEntry(metRequires, planEntry) {
			out = append(out, planEntry)
		}
	}
	return BuildPlan{Entries: out}
}

func containsEntry(metRequires []string, entry BuildPlanEntry) bool {
	for _, met := range metRequires {
		for _, planReq := range entry.Requires {
			if met == planReq.Name {
				return true
			}
		}
	}
	return false
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

func (bom *BOMEntry) convertMetadataToVersion() {
	if version, ok := bom.Metadata["version"]; ok {
		metadataVersion := fmt.Sprintf("%v", version)
		bom.Version = metadataVersion
	}
}

func (bom *BOMEntry) convertVersionToMetadata() {
	if bom.Version != "" {
		if bom.Metadata == nil {
			bom.Metadata = make(map[string]interface{})
		}
		bom.Metadata["version"] = bom.Version
		bom.Version = ""
	}
}
