package lifecycle

import (
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/platform"
)

type CacheMetadataRetriever interface {
	RetrieveFrom(cache Cache) (platform.CacheMetadata, error)
}

type DefaultCacheMetadataRetriever struct {
	Logger Logger
}

func NewCacheMetadataRetriever(logger Logger) CacheMetadataRetriever {
	return &DefaultCacheMetadataRetriever{
		Logger: logger,
	}
}

func (mr *DefaultCacheMetadataRetriever) RetrieveFrom(cache Cache) (platform.CacheMetadata, error) {
	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta platform.CacheMetadata
	if cache != nil {
		var err error
		if !cache.Exists() {
			mr.Logger.Info("Layer cache not found")
		}
		cacheMeta, err = cache.RetrieveMetadata()
		if err != nil {
			return cacheMeta, errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		mr.Logger.Debug("Usable cache not provided, using empty cache metadata")
	}

	return cacheMeta, nil
}
