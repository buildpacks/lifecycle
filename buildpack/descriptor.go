// Buildpack descriptor file (https://github.com/buildpacks/spec/blob/main/buildpack.md#buildpacktoml-toml).

package buildpack

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	KindBuildpack = "Buildpack"
	KindExtension = "Extension"
)

type BuildModule interface {
	Build(bpPlan Plan, config BuildConfig, bpEnv BuildEnv) (BuildResult, error)
	ConfigFile() *Descriptor
	Detect(config *DetectConfig, bpEnv BuildEnv) DetectRun
}

func ReadDescriptor(path string) (*Descriptor, error) {
	var (
		descriptor *Descriptor
		err        error
	)
	if _, err = toml.DecodeFile(path, &descriptor); err != nil {
		return &Descriptor{}, err
	}
	if descriptor.Dir, err = filepath.Abs(filepath.Dir(path)); err != nil {
		return &Descriptor{}, err
	}
	return descriptor, nil
}

type Descriptor struct {
	API       string `toml:"api"`
	Buildpack Info   `toml:"buildpack"` // exactly one of 'buildpack' or 'extension' must be populated
	Extension Info   `toml:"extension"` // exactly one of 'buildpack' or 'extension' must be populated
	Order     Order  `toml:"order"`
	Dir       string `toml:"-"`
}

func (b *Descriptor) ConfigFile() *Descriptor {
	return b
}

func (b *Descriptor) Info() *Info {
	switch {
	case b.IsBuildpack():
		return &b.Buildpack
	case b.IsExtension():
		return &b.Extension
	}
	return &Info{}
}

func (b *Descriptor) IsBuildpack() bool {
	return b.Buildpack.ID != ""
}

func (b *Descriptor) IsComposite() bool {
	return len(b.Order) > 0
}

func (b *Descriptor) IsExtension() bool {
	return b.Extension.ID != ""
}

func (b *Descriptor) Kind() string {
	if b.IsExtension() {
		return "extension"
	}
	return "buildpack"
}

func (b *Descriptor) String() string {
	return b.Buildpack.Name + " " + b.Buildpack.Version
}

type Info struct {
	ClearEnv bool     `toml:"clear-env,omitempty"`
	Homepage string   `toml:"homepage,omitempty"`
	ID       string   `toml:"id"`
	Name     string   `toml:"name"`
	Version  string   `toml:"version"`
	SBOM     []string `toml:"sbom-formats,omitempty" json:"sbom-formats,omitempty"`
}

type Order []Group

type Group struct {
	Group []GroupElement `toml:"group"`
}

func (bg Group) Append(group ...Group) Group {
	for _, g := range group {
		bg.Group = append(bg.Group, g.Group...)
	}
	return bg
}

// A GroupElement represents a buildpack referenced in a buildpack.toml's [[order.group]] OR
// a buildpack or extension in order.toml OR a buildpack or extension in group.toml.
type GroupElement struct {
	// Fields that are common to order.toml and group.toml

	// ID specifies the ID of the buildpack or extension.
	ID string `toml:"id" json:"id"`
	// Version specifies the version of the buildpack or extension.
	Version string `toml:"version" json:"version"`

	// Fields that are in group.toml only

	// API specifies the Buildpack API that the buildpack or extension implements.
	API string `toml:"api,omitempty" json:"-"`
	// Homepage specifies the homepage of the buildpack or extension.
	Homepage string `toml:"homepage,omitempty" json:"homepage,omitempty"`
	// Extension specifies whether the group element is a buildpack or an extension.
	Extension bool `toml:"extension,omitempty" json:"-"`

	// Fields that are in order.toml only

	// Optional specifies that the buildpack or extension is optional. Extensions are always optional.
	Optional bool `toml:"optional,omitempty" json:"optional,omitempty"`

	// Fields that are never written

	// OrderExt holds the order for extensions during the detect phase.
	OrderExt Order `toml:"-" json:"-"`
}

func (e GroupElement) Equals(o GroupElement) bool {
	return e.ID == o.ID &&
		e.Version == o.Version &&
		e.API == o.API &&
		e.Homepage == o.Homepage &&
		e.Extension == o.Extension &&
		e.Optional == o.Optional
}

func (e GroupElement) IsExtensionsOrder() bool {
	return len(e.OrderExt) > 0
}

func (e GroupElement) Kind() string {
	if e.Extension {
		return KindExtension
	}
	return KindBuildpack
}

func (e GroupElement) NoAPI() GroupElement {
	e.API = ""
	return e
}

func (e GroupElement) NoHomepage() GroupElement {
	e.Homepage = ""
	return e
}

func (e GroupElement) NoOpt() GroupElement {
	e.Optional = false
	return e
}

func (e GroupElement) String() string {
	return e.ID + "@" + e.Version
}

func (e GroupElement) WithAPI(version string) GroupElement {
	e.API = version
	return e
}

func (e GroupElement) WithHomepage(address string) GroupElement {
	e.Homepage = address
	return e
}
