package lifecycle

import "io"

//go:generate mockgen -package testmock -destination testmock/cache.go github.com/buildpack/lifecycle Cache
type Cache interface {
	Name() string
	SetMetadata(metadata CacheMetadata) error
	RetrieveMetadata() (CacheMetadata, bool, error)
	AddLayer(identifier string, sha string, tarPath string) error
	ReuseLayer(identifier string, sha string) error
	RetrieveLayer(sha string) (io.ReadCloser, error)
	Commit() error
}
