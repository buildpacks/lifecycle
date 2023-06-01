package platform

import (
	"errors"
	"fmt"

	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform/files"
)

const (
	TargetLabel                = "io.buildpacks.id"
	OSDistributionNameLabel    = "io.buildpacks.distribution.name"
	OSDistributionVersionLabel = "io.buildpacks.distribution.version"
)

func GetRunImageForExport(inputs LifecycleInputs) (files.RunImageForExport, error) {
	if inputs.PlatformAPI.LessThan("0.12") {
		stackMD, err := files.ReadStack(inputs.StackPath, cmd.DefaultLogger)
		if err != nil {
			return files.RunImageForExport{}, err
		}
		return stackMD.RunImage, nil
	}
	runMD, err := files.ReadRun(inputs.RunPath, cmd.DefaultLogger)
	if err != nil {
		return files.RunImageForExport{}, err
	}
	if len(runMD.Images) == 0 {
		return files.RunImageForExport{}, err
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
	buildMD := &files.BuildMetadata{}
	if err = files.DecodeBuildMetadata(launch.GetMetadataFilePath(inputs.LayersDir), inputs.PlatformAPI, buildMD); err != nil {
		return files.RunImageForExport{}, err
	}
	if len(buildMD.Extensions) > 0 {
		// Extensions could have switched the run image, so we can't assume the first run image in run.toml was intended
		return files.RunImageForExport{Image: inputs.RunImageRef}, nil
	}
	return runMD.Images[0], nil
}

func BestRunImageMirrorFor(targetRegistry string, runImageMD files.RunImageForExport, accessChecker ImageStrategy) (string, error) {
	var runImageMirrors []string
	if runImageMD.Image == "" {
		return "", errors.New("missing run image metadata")
	}
	runImageMirrors = append(runImageMirrors, runImageMD.Image)
	runImageMirrors = append(runImageMirrors, runImageMD.Mirrors...)

	keychain, err := auth.DefaultKeychain(runImageMirrors...)
	if err != nil {
		return "", fmt.Errorf("unable to create keychain: %w", err)
	}

	// Try to select run image on the same registry as the target
	runImageRef := byRegistry(targetRegistry, runImageMirrors, accessChecker, keychain)
	if runImageRef != "" {
		return runImageRef, nil
	}

	// Select the first run image we have access to
	for _, image := range runImageMirrors {
		ok, err := accessChecker.CheckReadAccess(image, keychain)
		if err != nil {
			return "", err
		}
		if ok {
			return image, nil
		}
	}

	return "", errors.New("failed to find accessible run image")
}

func byRegistry(reg string, repos []string, accessChecker ImageStrategy, keychain authn.Keychain) string {
	for _, repo := range repos {
		ref, err := name.ParseReference(repo, name.WeakValidation)
		if err != nil {
			continue
		}
		if reg == ref.Context().RegistryStr() {
			ok, err := accessChecker.CheckReadAccess(repo, keychain)
			if err != nil {
				return ""
			}
			if ok {
				return repo
			}
		}
	}
	return ""
}

type ImageStrategy interface {
	CheckReadAccess(repo string, keychain authn.Keychain) (bool, error)
}

type RemoteImageStrategy struct{}

func (s *RemoteImageStrategy) CheckReadAccess(repo string, keychain authn.Keychain) (bool, error) {
	img, err := remote.NewImage(repo, keychain)
	if err != nil {
		return false, fmt.Errorf("failed to get remote image: %w", err)
	}
	return img.CheckReadAccess(), nil
}

type NopImageStrategy struct{}

func (a *NopImageStrategy) CheckReadAccess(_ string, _ authn.Keychain) (bool, error) {
	return true, nil
}
