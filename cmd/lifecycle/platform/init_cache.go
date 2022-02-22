package platform

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cache"
)

func initCache(cacheImageRef, cacheDir string, keychain authn.Keychain) (lifecycle.Cache, error) {
	if cacheImageRef != "" {
		return initImageCache(cacheImageRef, keychain)
	}
	if cacheDir != "" {
		return initVolumeCache(cacheDir)
	}
	return nil, nil
}

func initImageCache(cacheImageRef string, keychain authn.Keychain) (lifecycle.Cache, error) {
	cacheStore, err := cache.NewImageCacheFromName(cacheImageRef, keychain)
	if err != nil {
		return nil, errors.Wrap(err, "creating image cache")
	}
	return cacheStore, nil
}

func initVolumeCache(cacheDir string) (lifecycle.Cache, error) {
	cacheStore, err := cache.NewVolumeCache(cacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "creating volume cache")
	}
	return cacheStore, nil
}
