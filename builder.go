package lifecycle

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"

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

type BuildpackStore interface {
	Lookup(bpID, bpVersion string) (Buildpack, error)
}

type Buildpack interface {
	Build(bpPlan BuildpackPlan, config BuildConfig) (BuildResult, error)
}

type BuildConfig struct {
	Env         BuildEnv
	AppDir      string
	PlatformDir string
	LayersDir   string
	Out         io.Writer
	Err         io.Writer
}

type BuildResult struct {
	BOM         []BOMEntry
	Labels      []Label
	MetRequires []string
	Processes   []launch.Process
	Slices      []layers.Slice
}

type BOMEntry struct {
	Require
	Buildpack GroupBuildpack `toml:"buildpack" json:"buildpack"`
}

type Label struct {
	Key   string `toml:"key"`
	Value string `toml:"value"`
}

type BuildpackPlan struct {
	Entries []Require `toml:"entries"`
}

type Builder struct {
	AppDir         string
	LayersDir      string
	PlatformDir    string
	PlatformAPI    *api.Version
	Env            BuildEnv
	Group          BuildpackGroup
	Plan           BuildPlan
	Out, Err       io.Writer
	BuildpackStore BuildpackStore
}

func (b *Builder) Build() (*BuildMetadata, error) {
	config, err := b.BuildConfig()
	if err != nil {
		return nil, err
	}

	processMap := newProcessMap()
	plan := b.Plan
	var bom []BOMEntry
	var slices []layers.Slice
	var labels []Label

	for _, bp := range b.Group.Group {
		bpTOML, err := b.BuildpackStore.Lookup(bp.ID, bp.Version)
		if err != nil {
			return nil, err
		}

		bpPlan := plan.find(bp.ID)
		br, err := bpTOML.Build(bpPlan, config)
		if err != nil {
			return nil, err
		}

		updateDefaultProcesses(br.Processes, api.MustParse(bp.API), b.PlatformAPI)

		bom = append(bom, br.BOM...)
		labels = append(labels, br.Labels...)
		plan = plan.filter(br.MetRequires)

		warning := processMap.add(br.Processes)

		if warning != "" {
			if _, err := b.Out.Write([]byte(warning)); err != nil {
				return nil, err
			}
		}
		slices = append(slices, br.Slices...)
	}

	if b.PlatformAPI.Compare(api.MustParse("0.4")) < 0 { // PlatformAPI <= 0.3
		for i := range bom {
			bom[i].convertMetadataToVersion()
		}
	}
	procList := processMap.list()

	return &BuildMetadata{
		BOM:                         bom,
		Buildpacks:                  b.Group.Group,
		Labels:                      labels,
		Processes:                   procList,
		Slices:                      slices,
		BuildpackDefaultProcessType: processMap.defaultType,
	}, nil
}

// we set default = true for web processes when platformAPI >= 0.6 and buildpackAPI < 0.6
func updateDefaultProcesses(processes []launch.Process, buildpackAPI *api.Version, platformAPI *api.Version) {
	if platformAPI.Compare(api.MustParse("0.6")) < 0 || buildpackAPI.Compare(api.MustParse("0.6")) >= 0 {
		return
	}

	for i := range processes {
		if processes[i].Type == "web" {
			processes[i].Default = true
		}
	}
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

	return BuildConfig{
		Env:         b.Env,
		AppDir:      appDir,
		PlatformDir: platformDir,
		LayersDir:   layersDir,
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

type processMap struct {
	typeToProcess map[string]launch.Process
	defaultType   string
}

func newProcessMap() processMap {
	return processMap{
		typeToProcess: make(map[string]launch.Process),
		defaultType:   "",
	}
}

// This function adds the processes from listToAdd to processMap
// it sets m.defaultType to the last default process
// if a non-default process overrides a default process, it returns a warning and unset m.defaultType
func (m *processMap) add(listToAdd []launch.Process) string {
	warning := ""
	for _, procToAdd := range listToAdd {
		if procToAdd.Default {
			m.defaultType = procToAdd.Type
			warning = ""
		} else if procToAdd.Type == m.defaultType {
			// non-default process overrides a default process
			m.defaultType = ""
			warning = fmt.Sprintf("Warning: redefining the following default process type with a process not marked as default: %s\n", procToAdd.Type)
		}
		m.typeToProcess[procToAdd.Type] = procToAdd
	}
	return warning
}

// list returns a sorted array of processes.
// The array is sorted based on the process types.
// The list is sorted for reproducibility.
func (m processMap) list() []launch.Process {
	var keys []string
	for proc := range m.typeToProcess {
		keys = append(keys, proc)
	}
	sort.Strings(keys)
	result := []launch.Process{}
	for _, key := range keys {
		result = append(result, m.typeToProcess[key].NoDefault()) // we set the default to false so it won't be part of metadata.toml
	}
	return result
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
