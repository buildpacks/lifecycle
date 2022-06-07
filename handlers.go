package lifecycle

import (
	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
)

//go:generate mockgen -package testmock -destination testmock/cache_handler.go github.com/buildpacks/lifecycle CacheHandler
type CacheHandler interface {
	InitCache(imageRef, dir string) (Cache, error)
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

//go:generate mockgen -package testmock -destination testmock/api_verifier.go github.com/buildpacks/lifecycle APIVerifier
// APIVerifier exists to avoid having the lifecycle package depend on the cmd package.
// This package dependency actually already exists, but we are trying to avoid making it worse.
// Eventually, much logic in the cmd package should move to the platform package, after which
// we might be able to remove this interface.
type APIVerifier interface {
	VerifyBuildpackAPI(kind, name, requested string) error
	VerifyBuildpackAPIsForGroup(group []buildpack.GroupElement) error
}

//go:generate mockgen -package testmock -destination testmock/config_handler.go github.com/buildpacks/lifecycle ConfigHandler
type ConfigHandler interface {
	ReadGroup(path string) ([]buildpack.GroupElement, error)
	ReadOrder(path string) (buildpack.Order, buildpack.Order, error)
}

type DefaultConfigHandler struct{}

func NewConfigHandler() *DefaultConfigHandler {
	return &DefaultConfigHandler{}
}

func (h *DefaultConfigHandler) ReadGroup(path string) ([]buildpack.GroupElement, error) {
	group, err := ReadGroup(path)
	if err != nil {
		return nil, errors.Wrap(err, "reading buildpack group")
	}
	return group.Group, nil
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
		Order    buildpack.Order `toml:"order"`
		OrderExt buildpack.Order `toml:"order-ext"`
	}
	_, err := toml.DecodeFile(path, &order)
	if err != nil {
		return nil, nil, errors.Wrap(err, "reading buildpack order file")
	}
	return order.Order, order.OrderExt, err
}
