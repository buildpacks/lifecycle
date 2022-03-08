package inputs

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cache"
)

//go:generate mockgen -package testmock -destination testmock/cache_handler.go github.com/buildpacks/lifecycle/platform/inputs CacheHandler
type CacheHandler interface {
	InitImageCache(cacheImageRef string) (lifecycle.Cache, error)
	InitVolumeCache(cacheDir string) (lifecycle.Cache, error)
}

type DefaultCacheHandler struct {
	keychain authn.Keychain
}

func NewCacheHandler(keychain authn.Keychain) *DefaultCacheHandler {
	return &DefaultCacheHandler{
		keychain: keychain,
	}
}

func (ch *DefaultCacheHandler) InitImageCache(cacheImageRef string) (lifecycle.Cache, error) {
	cacheStore, err := cache.NewImageCacheFromName(cacheImageRef, ch.keychain)
	if err != nil {
		return nil, errors.Wrap(err, "creating image cache")
	}
	return cacheStore, nil
}

func (ch *DefaultCacheHandler) InitVolumeCache(cacheDir string) (lifecycle.Cache, error) {
	cacheStore, err := cache.NewVolumeCache(cacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "creating volume cache")
	}
	return cacheStore, nil
}
