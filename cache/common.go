package cache

import (
	"errors"
	"fmt"

	"github.com/buildpack/imgutil/remote"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/auth"
	"github.com/buildpack/lifecycle/cmd"
)

var errCacheCommitted = errors.New("cache cannot be modified after commit")

// MaybeCache returns a Cache object if one can be created from given arguments or nil.
func MaybeCache(image, dir string) (lifecycle.Cache, error) {
	var c lifecycle.Cache
	if image != "" {
		origImage, err := remote.NewImage(
			image,
			auth.EnvKeychain(cmd.EnvRegistryAuth),
			remote.FromBaseImage(image),
		)
		if err != nil {
			return nil, fmt.Errorf("accessing cache image %q: %v", image, err)
		}
		emptyImage, err := remote.NewImage(
			image,
			auth.EnvKeychain(cmd.EnvRegistryAuth),
			remote.WithPreviousImage(image),
		)
		if err != nil {
			return nil, fmt.Errorf("creating new cache image %q: %v", image, err)
		}
		c = NewImageCache(origImage, emptyImage)
	} else if dir != "" {
		var err error
		c, err = NewVolumeCache(dir)
		if err != nil {
			return nil, cmd.FailErr(err, "create volume cache")
		}
	}
	return c, nil
}
