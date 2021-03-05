package layertypes

import "os"

type EncoderDecoder interface {
	IsSupported(buildpackAPI string) bool
	Encode(file *os.File, lmf LayerMetadataFile) error
	Decode(path string) (LayerMetadataFile, string, error)
}

type LayerMetadataFile struct {
	Data   interface{} `json:"data" toml:"metadata"`
	Build  bool        `json:"build" toml:"build"`
	Launch bool        `json:"launch" toml:"launch"`
	Cache  bool        `json:"cache" toml:"cache"`
}

func (lmf *LayerMetadataFile) UnsetFlags() {
	lmf.Launch = false
	lmf.Cache = false
	lmf.Build = false
}
