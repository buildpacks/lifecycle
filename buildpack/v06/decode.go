package v06

import (
	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack/types"
)

type typesTable struct {
	Build  bool `toml:"build"`
	Launch bool `toml:"launch"`
	Cache  bool `toml:"cache"`
}

type layerMetadataTomlFile struct {
	Data  interface{} `toml:"metadata"`
	Types typesTable  `toml:"types"`
}

type Decoder struct {
}

func NewDecoder() *Decoder {
	return &Decoder{}
}

func (d *Decoder) CanDecode(buildpackAPI string) bool {
	return api.MustParse(buildpackAPI).Compare(api.MustParse("0.6")) >= 0
}

func (d *Decoder) Decode(path string) (types.LayerMetadataFile, bool, error) {
	var lmtf layerMetadataTomlFile
	md, err := toml.DecodeFile(path, &lmtf)
	if err != nil {
		return types.LayerMetadataFile{}, true, err
	}
	isWrongFormat := typesInTopLevel(md)
	return types.LayerMetadataFile{Data: lmtf.Data, Build: lmtf.Types.Build, Launch: lmtf.Types.Launch, Cache: lmtf.Types.Cache}, !isWrongFormat, nil
}

func typesInTopLevel(md toml.MetaData) bool {
	return md.IsDefined("build") || md.IsDefined("launch") || md.IsDefined("cache")
}
