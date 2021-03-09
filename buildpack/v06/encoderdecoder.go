package v06

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack/layertypes"
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

type encoderDecoder06 struct {
}

func NewEncoderDecoder() layertypes.EncoderDecoder {
	return &encoderDecoder06{}
}

func (d *encoderDecoder06) IsSupported(buildpackAPI string) bool {
	return api.MustParse(buildpackAPI).Compare(api.MustParse("0.6")) >= 0
}

func unsetFlags(lmf *layertypes.LayerMetadataFile) {
	lmf.Build = false
	lmf.Cache = false
	lmf.Launch = false
}

func (d *encoderDecoder06) Encode(file *os.File, lmf layertypes.LayerMetadataFile) error {
	unsetFlags(&lmf)
	types := typesTable{Build: lmf.Build, Launch: lmf.Launch, Cache: lmf.Cache}
	lmtf := layerMetadataTomlFile{Data: lmf.Data, Types: types}
	return toml.NewEncoder(file).Encode(lmtf)
}

func (d *encoderDecoder06) Decode(path string) (layertypes.LayerMetadataFile, string, error) {
	var lmtf layerMetadataTomlFile
	md, err := toml.DecodeFile(path, &lmtf)
	if err != nil {
		return layertypes.LayerMetadataFile{}, "", err
	}
	msg := ""
	if isWrongFormat := typesInTopLevel(md); isWrongFormat {
		msg = fmt.Sprintf("the launch, cache and build flags should be in the types table of %s", path)
	}
	return layertypes.LayerMetadataFile{Data: lmtf.Data, Build: lmtf.Types.Build, Launch: lmtf.Types.Launch, Cache: lmtf.Types.Cache}, msg, nil
}

func typesInTopLevel(md toml.MetaData) bool {
	return md.IsDefined("build") || md.IsDefined("launch") || md.IsDefined("cache")
}
