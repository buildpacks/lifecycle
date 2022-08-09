package lifecycle

import (
	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/log"
)

//go:generate mockgen -package testmock -destination testmock/cache_handler.go github.com/buildpacks/lifecycle CacheHandler
type CacheHandler interface {
	InitCache(imageRef, dir string) (Cache, error)
}

//go:generate mockgen -package testmock -destination testmock/dir_store.go github.com/buildpacks/lifecycle DirStore
type DirStore interface {
	Lookup(kind, id, version string) (buildpack.BuildModule, error)
}

//go:generate mockgen -package testmock -destination testmock/image_handler.go github.com/buildpacks/lifecycle ImageHandler
type ImageHandler interface {
	InitImage(imageRef string) (imgutil.Image, error)
	Docker() bool
}

//go:generate mockgen -package testmock -destination testmock/registry_handler.go github.com/buildpacks/lifecycle RegistryHandler
type RegistryHandler interface {
	EnsureReadAccess(imageRefs ...string) error
	EnsureWriteAccess(imageRefs ...string) error
}

//go:generate mockgen -package testmock -destination testmock/buildpack_api_verifier.go github.com/buildpacks/lifecycle BuildpackAPIVerifier
type BuildpackAPIVerifier interface {
	VerifyBuildpackAPI(kind, name, requested string, logger log.Logger) error
}

//go:generate mockgen -package testmock -destination testmock/config_handler.go github.com/buildpacks/lifecycle ConfigHandler
type ConfigHandler interface {
	ReadGroup(path string) (buildpackGroup []buildpack.GroupElement, extensionsGroup []buildpack.GroupElement, err error)
	ReadOrder(path string) (buildpack.Order, buildpack.Order, error)
}

type DefaultConfigHandler struct{}

func NewConfigHandler() *DefaultConfigHandler {
	return &DefaultConfigHandler{}
}

func (h *DefaultConfigHandler) ReadGroup(path string) (buildpackGroup []buildpack.GroupElement, extensionsGroup []buildpack.GroupElement, err error) {
	var groupFile buildpack.Group
	groupFile, err = ReadGroup(path)
	if err != nil {
		return nil, nil, errors.Wrap(err, "reading buildpack group")
	}
	return groupFile.Group, groupFile.GroupExtensions, nil
}

func ReadGroup(path string) (buildpack.Group, error) {
	var group buildpack.Group
	_, err := toml.DecodeFile(path, &group)
	return group, err
}

func (h *DefaultConfigHandler) ReadOrder(path string) (buildpack.Order, buildpack.Order, error) {
	orderBp, orderExt, err := ReadOrder(path)
	if err != nil {
		return buildpack.Order{}, buildpack.Order{}, err
	}
	return orderBp, orderExt, nil
}

func ReadOrder(path string) (buildpack.Order, buildpack.Order, error) {
	var order struct {
		Order           buildpack.Order `toml:"order"`
		OrderExtensions buildpack.Order `toml:"order-extensions"`
	}
	_, err := toml.DecodeFile(path, &order)
	if err != nil {
		return nil, nil, errors.Wrap(err, "reading buildpack order file")
	}
	for g, group := range order.OrderExtensions {
		for e := range group.Group {
			group.Group[e].Extension = true
		}
		order.OrderExtensions[g] = group
	}
	return order.Order, order.OrderExtensions, err
}
