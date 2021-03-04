// Data Format Files for the buildpack api spec (https://github.com/buildpacks/spec/blob/main/buildpack.md#data-format).

package buildpack

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
)

// launch.toml

type LaunchTOML struct {
	BOM       []BOMEntry
	Labels    []Label
	Processes []launch.Process `toml:"processes"`
	Slices    []layers.Slice   `toml:"slices"`
}

type BOMEntry struct {
	Require
	Buildpack GroupBuildpack `toml:"buildpack" json:"buildpack"`
}

type Require struct {
	Name     string                 `toml:"name" json:"name"`
	Version  string                 `toml:"version,omitempty" json:"version,omitempty"`
	Metadata map[string]interface{} `toml:"metadata" json:"metadata"`
}

func (r *Require) convertMetadataToVersion() {
	if version, ok := r.Metadata["version"]; ok {
		r.Version = fmt.Sprintf("%v", version)
	}
}

func (r *Require) ConvertVersionToMetadata() {
	if r.Version != "" {
		if r.Metadata == nil {
			r.Metadata = make(map[string]interface{})
		}
		r.Metadata["version"] = r.Version
		r.Version = ""
	}
}

func (r *Require) hasDoublySpecifiedVersions() bool {
	if _, ok := r.Metadata["version"]; ok {
		return r.Version != ""
	}
	return false
}

func (r *Require) hasInconsistentVersions() bool {
	if version, ok := r.Metadata["version"]; ok {
		return r.Version != "" && r.Version != version
	}
	return false
}

func (r *Require) hasTopLevelVersions() bool {
	return r.Version != ""
}

type Label struct {
	Key   string `toml:"key"`
	Value string `toml:"value"`
}

// build.toml

type BuildTOML struct {
	BOM   []BOMEntry `toml:"bom"`
	Unmet []Unmet    `toml:"unmet"`
}

type Unmet struct {
	Name string `toml:"name"`
}

// store.toml

type StoreTOML struct {
	Data map[string]interface{} `json:"metadata" toml:"metadata"`
}

// build plan

type BuildPlan struct {
	PlanSections
	Or planSectionsList `toml:"or"`
}

func (p *PlanSections) hasInconsistentVersions() bool {
	for _, req := range p.Requires {
		if req.hasInconsistentVersions() {
			return true
		}
	}
	return false
}

func (p *PlanSections) hasDoublySpecifiedVersions() bool {
	for _, req := range p.Requires {
		if req.hasDoublySpecifiedVersions() {
			return true
		}
	}
	return false
}

func (p *PlanSections) hasTopLevelVersions() bool {
	for _, req := range p.Requires {
		if req.hasTopLevelVersions() {
			return true
		}
	}
	return false
}

type planSectionsList []PlanSections

func (p *planSectionsList) hasInconsistentVersions() bool {
	for _, planSection := range *p {
		if planSection.hasInconsistentVersions() {
			return true
		}
	}
	return false
}

func (p *planSectionsList) hasDoublySpecifiedVersions() bool {
	for _, planSection := range *p {
		if planSection.hasDoublySpecifiedVersions() {
			return true
		}
	}
	return false
}

func (p *planSectionsList) hasTopLevelVersions() bool {
	for _, planSection := range *p {
		if planSection.hasTopLevelVersions() {
			return true
		}
	}
	return false
}

type PlanSections struct {
	Requires []Require `toml:"requires"`
	Provides []Provide `toml:"provides"`
}

type Provide struct {
	Name string `toml:"name"`
}

// buildpack plan

type Plan struct {
	Entries []Require `toml:"entries"`
}

func (p Plan) filter(unmet []Unmet) Plan {
	var out []Require
	for _, entry := range p.Entries {
		if !containsName(unmet, entry.Name) {
			out = append(out, entry)
		}
	}
	return Plan{Entries: out}
}

func (p Plan) toBOM() []BOMEntry {
	var bom []BOMEntry
	for _, entry := range p.Entries {
		bom = append(bom, BOMEntry{Require: entry})
	}
	return bom
}

func containsName(unmet []Unmet, name string) bool {
	for _, u := range unmet {
		if u.Name == name {
			return true
		}
	}
	return false
}

// layer content metadata

type LayerMetadataFile struct {
	Data   interface{} `json:"data" toml:"metadata"`
	Build  bool        `json:"build" toml:"build"`
	Launch bool        `json:"launch" toml:"launch"`
	Cache  bool        `json:"cache" toml:"cache"`
}

type typesTable struct {
	Build  bool `toml:"build"`
	Launch bool `toml:"launch"`
	Cache  bool `toml:"cache"`
}
type layerMetadataTomlFile struct {
	Data  interface{} `toml:"metadata"`
	Types typesTable  `toml:"types"`
}

func (lmf *LayerMetadataFile) EncodeFalseFlags(path, buildpackAPI string) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	lmf.unsetFlags()
	if supportsTypesTable(buildpackAPI) {
		types := typesTable{Build: lmf.Build, Launch: lmf.Launch, Cache: lmf.Cache}
		lmtf := layerMetadataTomlFile{Data: lmf.Data, Types: types}
		return toml.NewEncoder(fh).Encode(lmtf)
	}
	return toml.NewEncoder(fh).Encode(lmf)
}

func typesInTopLevel(md toml.MetaData) bool {
	return md.IsDefined("build") || md.IsDefined("launch") || md.IsDefined("cache")
}

func typesInTypesTable(md toml.MetaData) bool {
	return md.IsDefined("types")
}

func DecodeLayerMetadataFile(path, buildpackAPI string) (LayerMetadataFile, bool /*are types in the right format*/, error) {
	fh, err := os.Open(path)
	if os.IsNotExist(err) {
		return LayerMetadataFile{}, true, nil
	} else if err != nil {
		return LayerMetadataFile{}, true, err
	}
	defer fh.Close()

	if supportsTypesTable(buildpackAPI) {
		var lmtf layerMetadataTomlFile
		md, err := toml.DecodeFile(path, &lmtf)
		if err != nil {
			return LayerMetadataFile{}, true, err
		}
		isWrongFormat := typesInTopLevel(md)
		return LayerMetadataFile{Data: lmtf.Data, Build: lmtf.Types.Build, Launch: lmtf.Types.Launch, Cache: lmtf.Types.Cache}, !isWrongFormat, nil
	}
	var lmf LayerMetadataFile
	md, err := toml.DecodeFile(path, &lmf)
	if err != nil {
		return LayerMetadataFile{}, true, err
	}
	isWrongFormat := typesInTypesTable(md)
	return lmf, !isWrongFormat, nil
}

func isBuild(path, buildpackAPI string) bool {
	layerMetadataFile, _, err := DecodeLayerMetadataFile(path, buildpackAPI)
	return err == nil && layerMetadataFile.Build
}

func supportsTypesTable(buildpackAPI string) bool {
	return api.MustParse(buildpackAPI).Compare(api.MustParse("0.6")) >= 0
}

func (lmf *LayerMetadataFile) unsetFlags() {
	lmf.Launch = false
	lmf.Cache = false
	lmf.Build = false
}
