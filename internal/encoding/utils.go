package encoding

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func MarshalTOML(v interface{}) ([]byte, error) { // TODO: test
	buf := new(bytes.Buffer)
	encoder := toml.NewEncoder(buf)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func WriteJSON(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(data)
}

func WriteTOML(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(data)
}
