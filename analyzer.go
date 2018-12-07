package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/img"
)

type Analyzer struct {
	Buildpacks []*Buildpack
	In         []byte
	Out, Err   io.Writer
}

func (a *Analyzer) Analyze(repoStore img.Store, launchDir string) error {
	metadata, err := a.GetMetadata(repoStore)
	if err != nil {
		return err
	}
	if metadata != nil {
		return a.AnalyzeMetadata(launchDir, *metadata)
	}
	return nil
}

func (a *Analyzer) AnalyzeMetadata(launchDir string, config AppImageMetadata) error {
	buildpacks := a.buildpacks()
	for _, buildpack := range config.Buildpacks {
		if _, exist := buildpacks[buildpack.ID]; !exist {
			continue
		}
		for name, metadata := range buildpack.Layers {
			path := filepath.Join(launchDir, buildpack.ID, name+".toml")
			if err := writeTOML(path, metadata.Data); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *Analyzer) GetMetadata(repoStore img.Store) (*AppImageMetadata, error) {
	origImage, err := repoStore.Image()
	if err != nil {
		fmt.Fprintf(a.Out, "WARNING: skipping analyze, authenticating to registry failed: %s\n", err.Error())
		return nil, nil
	}
	if _, err := origImage.RawManifest(); err != nil {
		if remoteErr, ok := err.(*remote.Error); ok && len(remoteErr.Errors) > 0 {
			switch remoteErr.Errors[0].Code {
			case remote.UnauthorizedErrorCode, remote.ManifestUnknownErrorCode:
				fmt.Fprintf(a.Out, "WARNING: skipping analyze, image not found or requires authentication to access: %s\n", remoteErr.Error())
				return nil, nil
			}
		}
		fmt.Fprintf(a.Out, "ERROR accessing manifest\n")
		return nil, cmd.FailErr(err, "access manifest", repoStore.Ref().Name())
	}

	configFile, err := origImage.ConfigFile()
	if err != nil {
		return nil, cmd.FailErr(err, "image configfile", repoStore.Ref().Name())
	}
	label := configFile.Config.Labels[MetadataLabel]
	if label == "" {
		fmt.Fprintf(a.Out, "WARNING: skipping analyze, previous image metadata was not found\n")
		return nil, nil
	}
	metadata := &AppImageMetadata{}
	if err := json.Unmarshal([]byte(label), metadata); err != nil {
		fmt.Fprintf(a.Out, "WARNING: skipping analyze, previous image metadata was incompatible\n")
		return nil, nil
	}
	return metadata, nil
}

func (a *Analyzer) buildpacks() map[string]struct{} {
	buildpacks := make(map[string]struct{}, len(a.Buildpacks))
	for _, b := range a.Buildpacks {
		buildpacks[b.ID] = struct{}{}
	}
	return buildpacks
}

func writeTOML(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	return toml.NewEncoder(fh).Encode(data)
}
