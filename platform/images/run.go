package images

import (
	"github.com/buildpacks/imgutil"

	"github.com/buildpacks/lifecycle/platform/files"
)

const (
	TargetLabel                = "io.buildpacks.id"
	OSDistributionNameLabel    = "io.buildpacks.distribution.name"
	OSDistributionVersionLabel = "io.buildpacks.distribution.version"
)

func GetTargetMetadataFrom(image imgutil.Image) (*files.TargetMetadata, error) {
	tm := files.TargetMetadata{}
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
		tm.Distribution = &files.OSDistribution{Name: distName, Version: distVersion}
	}
	if id, exists := labels[TargetLabel]; exists {
		tm.ID = id
	}

	return &tm, nil
}
