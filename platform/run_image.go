package platform

import (
	"github.com/buildpacks/imgutil"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/launch"
)

const (
	TargetLabel                = "io.buildpacks.id"
	OSDistributionNameLabel    = "io.buildpacks.distribution.name"
	OSDistributionVersionLabel = "io.buildpacks.distribution.version"
)

func GetRunImageForExport(inputs LifecycleInputs) (RunImageForExport, error) {
	if inputs.PlatformAPI.LessThan("0.12") {
		stackMD, err := ReadStack(inputs.StackPath, cmd.DefaultLogger)
		if err != nil {
			return RunImageForExport{}, err
		}
		return stackMD.RunImage, nil
	}
	runMD, err := ReadRun(inputs.RunPath, cmd.DefaultLogger)
	if err != nil {
		return RunImageForExport{}, err
	}
	if len(runMD.Images) == 0 {
		return RunImageForExport{}, err
	}
	for _, runImage := range runMD.Images {
		if runImage.Image == inputs.RunImageRef {
			return runImage, nil
		}
		for _, mirror := range runImage.Mirrors {
			if mirror == inputs.RunImageRef {
				return runImage, nil
			}
		}
	}
	buildMD := &BuildMetadata{}
	if err = DecodeBuildMetadataTOML(launch.GetMetadataFilePath(inputs.LayersDir), inputs.PlatformAPI, buildMD); err != nil {
		return RunImageForExport{}, err
	}
	if len(buildMD.Extensions) > 0 {
		// Extensions could have switched the run image, so we can't assume the first run image in run.toml was intended
		return RunImageForExport{Image: inputs.RunImageRef}, nil
	}
	return runMD.Images[0], nil
}

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
