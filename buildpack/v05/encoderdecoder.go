package v05

import (
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack/layermetadata"
)

type EncoderDecoder05 struct {
}

func NewEncoderDecoder() *EncoderDecoder05 {
	return &EncoderDecoder05{}
}

func (d *EncoderDecoder05) IsSupported(buildpackAPI string) bool {
	return api.MustParse(buildpackAPI).LessThan("0.6")
}

func (d *EncoderDecoder05) Encode(file *os.File, lmf layermetadata.File) error {
	return toml.NewEncoder(file).Encode(lmf)
}

func (d *EncoderDecoder05) Decode(path string) (layermetadata.File, string, error) {
	var lmf layermetadata.File
	md, err := toml.DecodeFile(path, &lmf)
	if err != nil {
		return layermetadata.File{}, "", err
	}
	msg := ""
	if isWrongFormat := typesInTypesTable(md); isWrongFormat {
		msg = "Types table isn't supported in this buildpack api version. The launch, build and cache flags should be in the top level. Ignoring the values in the types table."
	}
	return lmf, msg, nil
}

func typesInTypesTable(md toml.MetaData) bool {
	return md.IsDefined("types")
}
