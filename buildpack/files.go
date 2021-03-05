// Data Format Files for the buildpack api spec (https://github.com/buildpacks/spec/blob/main/buildpack.md#data-format).

package buildpack

import (
	"errors"
	"fmt"
	"os"

	"github.com/buildpacks/lifecycle/buildpack/layertypes"
	v05 "github.com/buildpacks/lifecycle/buildpack/v05"
	v06 "github.com/buildpacks/lifecycle/buildpack/v06"
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

func DefaultEncodersDecoders() []layertypes.EncoderDecoder {
	return []layertypes.EncoderDecoder{
		v05.NewEncoderDecoder(),
		v06.NewEncoderDecoder(),
	}
}

func EncodeFalseFlags(lmf layertypes.LayerMetadataFile, path, buildpackAPI string) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	lmf.UnsetFlags()

	encoders := DefaultEncodersDecoders()

	for _, encoder := range encoders {
		if encoder.IsSupported(buildpackAPI) {
			return encoder.Encode(fh, lmf)
		}
	}
	return errors.New("couldn't find an encoder")
}

func DecodeLayerMetadataFile(path, buildpackAPI string) (layertypes.LayerMetadataFile, string /*warning/error*/, error) {
	fh, err := os.Open(path)
	if os.IsNotExist(err) {
		return layertypes.LayerMetadataFile{}, "", nil
	} else if err != nil {
		return layertypes.LayerMetadataFile{}, "", err
	}
	defer fh.Close()

	decoders := DefaultEncodersDecoders()

	for _, decoder := range decoders {
		if decoder.IsSupported(buildpackAPI) {
			return decoder.Decode(path)
		}
	}
	return layertypes.LayerMetadataFile{}, "", errors.New("couldn't find a decoder")
}

func isBuild(path, buildpackAPI string) bool {
	layerMetadataFile, _, err := DecodeLayerMetadataFile(path, buildpackAPI)
	return err == nil && layerMetadataFile.Build
}
