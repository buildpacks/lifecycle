package buildpack

import (
	"errors"
	"fmt"

	"github.com/buildpacks/lifecycle/api"
)

type BOMValidator interface {
	ValidateBOM([]BOMEntry) error
}

func NewBOMValidator(bpAPI string, logger Logger) BOMValidator {
	switch {
	case api.MustParse(bpAPI).LessThan("0.5"):
		return &LegacyBOMValidator{}
	case api.MustParse(bpAPI).LessThan("0.7"):
		return &StrictBOMValidator{}
	default:
		return &DefaultBOMValidator{Logger: logger}
	}
}

type DefaultBOMValidator struct {
	Logger Logger
}

func (v *DefaultBOMValidator) ValidateBOM(bom []BOMEntry) error {
	if len(bom) > 0 {
		v.Logger.Warn("BOM table isn't supported in this buildpack api version. The BOM should be written to <layer>.bom.<ext>, launch.bom.<ext>, or build.bom.<ext>.")
	}
	return nil
}

type LegacyBOMValidator struct{}

func (v *LegacyBOMValidator) ValidateBOM(bom []BOMEntry) error {
	for _, entry := range bom {
		if version, ok := entry.Metadata["version"]; ok {
			metadataVersion := fmt.Sprintf("%v", version)
			if entry.Version != "" && entry.Version != metadataVersion {
				return errors.New("top level version does not match metadata version")
			}
		}
	}
	return nil
}

type StrictBOMValidator struct{}

func (v *StrictBOMValidator) ValidateBOM(bom []BOMEntry) error {
	for _, entry := range bom {
		if entry.Version != "" {
			return fmt.Errorf("bom entry '%s' has a top level version which is not allowed. The buildpack should instead set metadata.version", entry.Name)
		}
	}
	return nil
}

type BOMHandler interface {
	HandleBOM(GroupBuildpack, []BOMEntry) []BOMEntry
}

func NewBOMHandler(bpAPI string) BOMHandler {
	switch {
	case api.MustParse(bpAPI).LessThan("0.5"):
		return &LegacyBOMHandler{}
	case api.MustParse(bpAPI).LessThan("0.7"):
		return &StrictBOMHandler{}
	default:
		return &DefaultBOMHandler{}
	}
}

type DefaultBOMHandler struct{}

func (h *DefaultBOMHandler) HandleBOM(_ GroupBuildpack, _ []BOMEntry) []BOMEntry {
	return []BOMEntry{}
}

type LegacyBOMHandler struct{}

func (h *LegacyBOMHandler) HandleBOM(buildpack GroupBuildpack, bom []BOMEntry) []BOMEntry {
	bom = WithBuildpack(buildpack, bom)
	for i := range bom {
		bom[i].convertVersionToMetadata()
	}
	return bom
}

type StrictBOMHandler struct{}

func (h *StrictBOMHandler) HandleBOM(buildpack GroupBuildpack, bom []BOMEntry) []BOMEntry {
	return WithBuildpack(buildpack, bom)
}
