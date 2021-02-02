package lifecycle

import (
	"container/list"
	"fmt"
	"io"
	"path/filepath"
	"strings"

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
	BuilpackAPI() string
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

	procStruct := newProcessStruct()
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

		updateDefaultProcesses(br.Processes, api.MustParse(bpTOML.BuilpackAPI()), b.PlatformAPI)

		bom = append(bom, br.BOM...)
		labels = append(labels, br.Labels...)
		plan = plan.filter(br.MetRequires)
		replacedDefaults, err := procStruct.add(br.Processes)
		if err != nil {
			return nil, err
		}

		if len(replacedDefaults) > 0 {
			warning := fmt.Sprintf("Warning: redefining the following default process types with processes not marked as default: [%s]", strings.Join(replacedDefaults, ", "))
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
	procList, err := procStruct.list()
	if err != nil {
		return nil, err
	}

	return &BuildMetadata{
		BOM:        bom,
		Buildpacks: b.Group.Group,
		Labels:     labels,
		Processes:  procList,
		Slices:     slices,
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

// orderedProcesses is a mapping from process types to Processes, it will preserve ordering.
// processList is the ordered list.
// we keep typeToProcess map in order to delete elements when others are overriding them.
type orderedProcesses struct {
	typeToProcess map[string]*list.Element
	processList   *list.List
}

func newProcessStruct() orderedProcesses {
	return orderedProcesses{
		typeToProcess: make(map[string]*list.Element),
		processList:   list.New(),
	}
}

func (m orderedProcesses) add(listToAdd []launch.Process) ([]string, error) {
	result := []string{}
	for _, proc := range listToAdd {
		// when we replace a default process type with a non-default process type, add to result
		if p, ok := m.typeToProcess[proc.Type]; ok {
			cast, success := (p.Value).(launch.Process)
			if !success {
				return []string{}, fmt.Errorf("can't cast an element from the list to a process")
			}
			if cast.Default && !proc.Default {
				result = append(result, proc.Type)
			}
			m.processList.Remove(p)
		}
		m.processList.PushBack(proc)
		m.typeToProcess[proc.Type] = m.processList.Back()
	}

	return result, nil
}

// list returns an ordered array of process types. The ordering is based on the
// order that the processes were added to this struct.
func (m orderedProcesses) list() ([]launch.Process, error) {
	result := []launch.Process{}
	for e := m.processList.Front(); e != nil; e = e.Next() {
		cast, success := (e.Value).(launch.Process)
		if !success {
			return []launch.Process{}, fmt.Errorf("can't cast an element from the list to a process")
		}
		result = append(result, cast)
	}
	return result, nil
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
