package layermetadata

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/api"
)

type EncoderDecoder interface {
	IsSupported(buildpackAPI string) bool
	Encode(file *os.File, lmf File) error
	Decode(path string) (File, string, error)
}

func supportedEncoderDecoders() []EncoderDecoder {
	return []EncoderDecoder{
		&DefaultEncoderDecoder{},
		&LegacyEncoderDecoder{},
	}
}

func EncodeFile(lmf File, path, buildpackAPI string) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	encoders := supportedEncoderDecoders()

	for _, encoder := range encoders {
		if encoder.IsSupported(buildpackAPI) {
			return encoder.Encode(fh, lmf)
		}
	}
	return errors.New("couldn't find an encoder")
}

func DecodeFile(path, buildpackAPI string) (File, string, error) { // TODO: pass the logger and print the warning inside (instead of returning a message)
	fh, err := os.Open(path)
	if os.IsNotExist(err) {
		return File{}, "", nil
	} else if err != nil {
		return File{}, "", err
	}
	defer fh.Close()

	decoders := supportedEncoderDecoders()

	for _, decoder := range decoders {
		if decoder.IsSupported(buildpackAPI) {
			return decoder.Decode(path)
		}
	}
	return File{}, "", errors.New("couldn't find a decoder")
}

type DefaultEncoderDecoder struct{}

func (d *DefaultEncoderDecoder) IsSupported(buildpackAPI string) bool {
	return api.MustParse(buildpackAPI).AtLeast("0.6")
}

func (d *DefaultEncoderDecoder) Encode(file *os.File, lmf File) error {
	// omit the types table - all the flags are set to false
	type dataTomlFile struct {
		Data interface{} `toml:"metadata"`
	}
	dtf := dataTomlFile{Data: lmf.Data}
	return toml.NewEncoder(file).Encode(dtf)
}

func (d *DefaultEncoderDecoder) Decode(path string) (File, string, error) {
	type typesTable struct {
		Build  bool `toml:"build"`
		Launch bool `toml:"launch"`
		Cache  bool `toml:"cache"`
	}
	type layerMetadataTomlFile struct {
		Data  interface{} `toml:"metadata"`
		Types typesTable  `toml:"types"`
	}

	var lmtf layerMetadataTomlFile
	md, err := toml.DecodeFile(path, &lmtf)
	if err != nil {
		return File{}, "", err
	}
	msg := ""
	if isWrongFormat := typesInTopLevel(md); isWrongFormat {
		msg = fmt.Sprintf("the launch, cache and build flags should be in the types table of %s", path)
	}
	return File{Data: lmtf.Data, Build: lmtf.Types.Build, Launch: lmtf.Types.Launch, Cache: lmtf.Types.Cache}, msg, nil
}

func typesInTopLevel(md toml.MetaData) bool {
	return md.IsDefined("build") || md.IsDefined("launch") || md.IsDefined("cache")
}

type LegacyEncoderDecoder struct{}

func (d *LegacyEncoderDecoder) IsSupported(buildpackAPI string) bool {
	return api.MustParse(buildpackAPI).LessThan("0.6")
}

func (d *LegacyEncoderDecoder) Encode(file *os.File, lmf File) error {
	return toml.NewEncoder(file).Encode(lmf)
}

func (d *LegacyEncoderDecoder) Decode(path string) (File, string, error) {
	var lmf File
	md, err := toml.DecodeFile(path, &lmf)
	if err != nil {
		return File{}, "", err
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
