package platform

import (
	"github.com/buildpacks/imgutil"
)

const (
	TargetLabel                = "io.buildpacks.id"
	OSDistributionNameLabel    = "io.buildpacks.distribution.name"
	OSDistributionVersionLabel = "io.buildpacks.distribution.version"
)

func GetTargetFromImage(image imgutil.Image) (*TargetMetadata, error) {
	tm := TargetMetadata{}
	if !image.Found() {
		return &tm, nil
	}
	var err error
	tm.OS, err = image.OS()
	if err != nil {
		return &tm, err
	}
	tm.Arch, err = image.Architecture()
	if err != nil {
		return &tm, err
	}
	tm.ArchVariant, err = image.Variant()
	if err != nil {
		return &tm, err
	}
	labels, err := image.Labels()
	if err != nil {
		return &tm, err
	}
	distName, distNameExists := labels[OSDistributionNameLabel]
	distVersion, distVersionExists := labels[OSDistributionVersionLabel]
	if distNameExists || distVersionExists {
		tm.Distribution = &OSDistribution{Name: distName, Version: distVersion}
	}
	if id, exists := labels[TargetLabel]; exists {
		tm.ID = id
	}

	return &tm, nil
}
