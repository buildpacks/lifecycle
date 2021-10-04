package buildpack

import (
	"errors"
	"fmt"

	"github.com/buildpacks/lifecycle/api"
)

type BOMValidator interface {
	ValidateBOM(GroupBuildpack, []BOMEntry) ([]BOMEntry, error)
}

func NewBOMValidator(bpAPI string, logger Logger) BOMValidator {
	switch {
	case api.MustParse(bpAPI).LessThan("0.5"):
		return &legacyBOMValidator{}
	case api.MustParse(bpAPI).LessThan("0.7"):
		return &v05To06BOMValidator{}
	default:
		return &defaultBOMValidator{logger: logger}
	}
}

type defaultBOMValidator struct {
	logger Logger
}

func (h *defaultBOMValidator) ValidateBOM(bp GroupBuildpack, bom []BOMEntry) ([]BOMEntry, error) {
	if err := h.validateBOM(bom); err != nil {
		return []BOMEntry{}, err
	}
	return h.processBOM(bp, bom), nil
}

func (h *defaultBOMValidator) validateBOM(bom []BOMEntry) error {
	if len(bom) > 0 {
		h.logger.Warn("BOM table isn't supported in this buildpack api version. The BOM should be written to <layer>.bom.<ext>, launch.bom.<ext>, or build.bom.<ext>.")
	}
	return nil
}

func (h *defaultBOMValidator) processBOM(_ GroupBuildpack, _ []BOMEntry) []BOMEntry {
	return []BOMEntry{}
}

type v05To06BOMValidator struct{}

func (h *v05To06BOMValidator) ValidateBOM(bp GroupBuildpack, bom []BOMEntry) ([]BOMEntry, error) {
	if err := h.validateBOM(bom); err != nil {
		return []BOMEntry{}, err
	}
	return h.processBOM(bp, bom), nil
}

func (h *v05To06BOMValidator) validateBOM(bom []BOMEntry) error {
	for _, entry := range bom {
		if entry.Version != "" {
			return fmt.Errorf("bom entry '%s' has a top level version which is not allowed. The buildpack should instead set metadata.version", entry.Name)
		}
	}
	return nil
}

func (h *v05To06BOMValidator) processBOM(buildpack GroupBuildpack, bom []BOMEntry) []BOMEntry {
	return WithBuildpack(buildpack, bom)
}

type legacyBOMValidator struct{}

func (h *legacyBOMValidator) ValidateBOM(bp GroupBuildpack, bom []BOMEntry) ([]BOMEntry, error) {
	if err := h.validateBOM(bom); err != nil {
		return []BOMEntry{}, err
	}
	return h.processBOM(bp, bom), nil
}

func (h *legacyBOMValidator) validateBOM(bom []BOMEntry) error {
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

func (h *legacyBOMValidator) processBOM(buildpack GroupBuildpack, bom []BOMEntry) []BOMEntry {
	bom = WithBuildpack(buildpack, bom)
	for i := range bom {
		bom[i].convertVersionToMetadata()
	}
	return bom
}
