package phase

import (
	"fmt"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/files"
)

var Config = &DefaultConfigHandler{}

// CacheHandler wraps initialization of a cache image or cache volume.
//
//go:generate mockgen -package testmock -destination testmock/cache_handler.go github.com/buildpacks/lifecycle/phase CacheHandler
type CacheHandler interface {
	InitCache(imageRef, dir string, deletionEnabled bool) (Cache, error)
}

// DirStore is a repository of buildpacks and/or image extensions.
// Each element should be present on disk according to the format outlined in the Platform Interface Specification,
// namely: `/cnb/<buildpacks|extensions>/<id>/<version>/<root directory>`.
//
//go:generate mockgen -package testmock -destination testmock/dir_store.go github.com/buildpacks/lifecycle/phase DirStore
type DirStore interface {
	Lookup(kind, id, version string) (buildpack.Descriptor, error)
	LookupBp(id, version string) (*buildpack.BpDescriptor, error)
	LookupExt(id, version string) (*buildpack.ExtDescriptor, error)
}

// BuildpackAPIVerifier verifies a requested Buildpack API version.
//
//go:generate mockgen -package testmock -destination testmock/buildpack_api_verifier.go github.com/buildpacks/lifecycle/phase BuildpackAPIVerifier
type BuildpackAPIVerifier interface {
	VerifyBuildpackAPI(kind, name, requestedVersion string, logger log.Logger) error
}

// ConfigHandler reads configuration files for the lifecycle.
//
//go:generate mockgen -package testmock -destination testmock/config_handler.go github.com/buildpacks/lifecycle/phase ConfigHandler
type ConfigHandler interface {
	ReadAnalyzed(path string, logger log.Logger) (files.Analyzed, error)
	ReadGroup(path string) (buildpackGroup []buildpack.GroupElement, extensionsGroup []buildpack.GroupElement, err error)
	ReadOrder(path string) (buildpack.Order, buildpack.Order, error)
	ReadRun(runPath string, logger log.Logger) (files.Run, error)
}

type DefaultConfigHandler struct{}

func NewConfigHandler() *DefaultConfigHandler {
	return &DefaultConfigHandler{}
}

func (h *DefaultConfigHandler) ReadAnalyzed(path string, logr log.Logger) (files.Analyzed, error) {
	return files.ReadAnalyzed(path, logr)
}

func (h *DefaultConfigHandler) ReadGroup(path string) (buildpackGroup []buildpack.GroupElement, extensionsGroup []buildpack.GroupElement, err error) {
	var groupFile buildpack.Group
	groupFile, err = ReadGroup(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read group file: %w", err)
	}
	return groupFile.Group, groupFile.GroupExtensions, nil
}

func ReadGroup(path string) (buildpack.Group, error) {
	var group buildpack.Group
	_, err := toml.DecodeFile(path, &group)
	for e := range group.GroupExtensions {
		group.GroupExtensions[e].Extension = true
		group.GroupExtensions[e].Optional = true
	}
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
		return nil, nil, fmt.Errorf("failed to read order file: %w", err)
	}
	for g, group := range order.OrderExtensions {
		for e := range group.Group {
			group.Group[e].Extension = true
			group.Group[e].Optional = true
		}
		order.OrderExtensions[g] = group
	}
	return order.Order, order.OrderExtensions, err
}

func (h *DefaultConfigHandler) ReadRun(runPath string, logger log.Logger) (files.Run, error) {
	return files.ReadRun(runPath, logger)
}
