// Buildpack descriptor file (https://github.com/buildpacks/spec/blob/main/buildpack.md#buildpacktoml-toml).

package buildpack

import "github.com/BurntSushi/toml"

type Descriptor struct {
	API       string `toml:"api"`
	Buildpack Info   `toml:"buildpack"`
	Order     Order  `toml:"order"`
	Dir       string `toml:"-"`
}

func (b *Descriptor) ConfigFile() *Descriptor {
	return b
}

func (b *Descriptor) IsMetaBuildpack() bool {
	return b.Order != nil
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
	Group []GroupBuildable `toml:"group"`
}

func (bg Group) Append(group ...Group) Group {
	for _, g := range group {
		bg.Group = append(bg.Group, g.Group...)
	}
	return bg
}

func (bg Group) Filter(useExtensions bool) Group {
	ret := Group{Group: []GroupBuildable{}}
	for _, gb := range bg.Group {
		if gb.Extension == useExtensions {
			ret.Group = append(ret.Group, gb)
		}
	}
	return ret
}

func ReadGroup(path string) (Group, error) {
	var group Group
	_, err := toml.DecodeFile(path, &group)
	return group, err
}

func ReadOrder(path string) (Order, Order, error) {
	var order struct { // TODO: move this maybe
		Order    Order `toml:"order"`
		OrderExt Order `toml:"order-ext"`
	}
	_, err := toml.DecodeFile(path, &order)
	return order.Order, order.OrderExt, err
}

// TODO: GroupElement is probably a better name for this
// TODO: this struct is the amalgamation of all fields from order.toml & group.toml, which is a bit confusing and ties the code together in weird ways

// A GroupBuildable represents a buildpack or extension referenced in a buildpack.toml's [[order.group]].
// A GroupBuildable buildpack may be a regular buildpack, or a meta buildpack.
type GroupBuildable struct {
	API       string `toml:"api,omitempty" json:"-"`                       // group.toml
	Homepage  string `toml:"homepage,omitempty" json:"homepage,omitempty"` // group.toml
	ID        string `toml:"id" json:"id"`
	Version   string `toml:"version" json:"version"`
	Extension bool   `toml:"extension" json:"extension"`                   // group.toml
	Optional  bool   `toml:"optional,omitempty" json:"optional,omitempty"` // order.toml
	OrderExt  Order  `toml:"-" json:"-"`                                   // only for use by the Detector
}

func (b GroupBuildable) String() string {
	return b.ID + "@" + b.Version
}

func (b GroupBuildable) NoOpt() GroupBuildable {
	b.Optional = false
	return b
}

func (b GroupBuildable) NoAPI() GroupBuildable {
	b.API = ""
	return b
}

func (b GroupBuildable) NoHomepage() GroupBuildable {
	b.Homepage = ""
	return b
}
