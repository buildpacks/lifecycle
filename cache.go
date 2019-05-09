package lifecycle

import (
	"io"

	"github.com/buildpack/lifecycle/cache"
)

//go:generate mockgen -package testmock -destination testmock/cache.go github.com/buildpack/lifecycle Cache
type Cache interface {
	Name() string
	SetMetadata(metadata cache.Metadata) error
	RetrieveMetadata() (cache.Metadata, error)
	AddLayerFile(sha string, tarPath string) error
	ReuseLayer(sha string) error
	RetrieveLayer(sha string) (io.ReadCloser, error)
	Commit() error
}
