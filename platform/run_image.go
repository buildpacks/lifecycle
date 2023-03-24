package platform

import (
	"encoding/json"
	"errors"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const (
	TargetLabel                = "io.buildpacks.target"
	OSDistributionNameLabel    = "io.buildpacks.distribution.name"
	OSDistributionVersionLabel = "io.buildpacks.distribution.version"
)

func ReadTargetData(fromImage v1.Image) (TargetMetadata, error) {
	var (
		targetID, os, arch, archVariant, distName, distVersion string
		err                                                    error
	)
	configFile, err := fromImage.ConfigFile()
	if err != nil {
		return TargetMetadata{}, err
	}
	if configFile == nil {
		return TargetMetadata{}, errors.New("missing image config")
	}
	if err = decodeOptionalLabel(configFile, TargetLabel, &targetID); err != nil {
		return TargetMetadata{}, err
	}
	os = configFile.OS
	arch = configFile.Architecture
	archVariant = configFile.Variant
	if err = decodeOptionalLabel(configFile, OSDistributionNameLabel, &distName); err != nil {
		return TargetMetadata{}, err
	}
	if err = decodeOptionalLabel(configFile, OSDistributionVersionLabel, &distVersion); err != nil {
		return TargetMetadata{}, err
	}
	return TargetMetadata{
		ID:          targetID,
		OS:          os,
		Arch:        arch,
		ArchVariant: archVariant,
		Distribution: &OSDistribution{
			Name:    distName,
			Version: distVersion,
		},
	}, nil
}

func decodeOptionalLabel(fromConfig *v1.ConfigFile, withName string, to interface{}) error {
	if fromConfig == nil {
		return errors.New("missing image config")
	}
	labels := fromConfig.Config.Labels
	contents, ok := labels[withName]
	if !ok {
		return nil
	}
	if contents == "" {
		return nil
	}
	return json.Unmarshal([]byte(contents), to)
}
