package cache

import (
	"errors"

	"github.com/buildpack/lifecycle/metadata"
)

const MetadataLabel = "io.buildpacks.lifecycle.cache.metadata"

var (
	errCacheCommitted = errors.New("cache cannot be modified after commit")
)

type Metadata struct {
	Buildpacks []metadata.BuildpackMetadata `json:"buildpacks"`
}

func (m *Metadata) MetadataForBuildpack(id string) metadata.BuildpackMetadata {
	for _, bpMd := range m.Buildpacks {
		if bpMd.ID == id {
			return bpMd
		}
	}
	return metadata.BuildpackMetadata{}
}
