package lifecycle

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
)

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

func ReadGroup(path string) (buildpack.Group, error) {
	var group buildpack.Group
	_, err := toml.DecodeFile(path, &group)
	return group, err
}

func ReadOrder(path string) (buildpack.Order, error) {
	var order struct {
		Order buildpack.Order `toml:"order"`
	}
	_, err := toml.DecodeFile(path, &order)
	return order.Order, err
}

func TruncateSha(sha string) string {
	rawSha := strings.TrimPrefix(sha, "sha256:")
	if len(sha) > 12 {
		return rawSha[0:12]
	}
	return rawSha
}

func DecodeLabel(image imgutil.Image, label string, v interface{}) error {
	if !image.Found() {
		return nil
	}
	contents, err := image.Label(label)
	if err != nil {
		return errors.Wrapf(err, "retrieving label '%s' for image '%s'", label, image.Name())
	}
	if contents == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(contents), v); err != nil {
		return errors.Wrapf(err, "failed to unmarshal context of label '%s'", label)
	}
	return nil
}

func Copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func removeStagePrefixes(mixins []string) []string {
	var result []string
	for _, m := range mixins {
		s := strings.SplitN(m, ":", 2)
		if len(s) == 1 {
			result = append(result, s[0])
		} else {
			result = append(result, s[1])
		}
	}
	return result
}
