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

type TargetData struct {
	ID           string          `toml:"id"`
	OS           string          `toml:"os"`
	Arch         string          `toml:"arch"`
	ArchVariant  string          `toml:"variant"`
	Distribution *OSDistribution `toml:"distribution"`
}

type OSDistribution struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
}

func ReadTargetData(fromImage v1.Image) (TargetData, error) {
	var (
		targetID, os, arch, archVariant, distName, distVersion string
		err                                                    error
	)
	configFile, err := fromImage.ConfigFile()
	if err != nil {
		return TargetData{}, err
	}
	if configFile == nil {
		return TargetData{}, errors.New("missing image config")
	}
	if err = decodeOptionalLabel(configFile, TargetLabel, &targetID); err != nil {
		return TargetData{}, err
	}
	os = configFile.OS
	arch = configFile.Architecture
	archVariant = configFile.Variant
	if err = decodeOptionalLabel(configFile, OSDistributionNameLabel, &distName); err != nil {
		return TargetData{}, err
	}
	if err = decodeOptionalLabel(configFile, OSDistributionVersionLabel, &distVersion); err != nil {
		return TargetData{}, err
	}
	return TargetData{
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
