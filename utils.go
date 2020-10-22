package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"
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

func ReadGroup(path string) (BuildpackGroup, error) {
	var group BuildpackGroup
	_, err := toml.DecodeFile(path, &group)
	return group, err
}

func ReadOrder(path string) (BuildpackOrder, error) {
	var order struct {
		Order BuildpackOrder `toml:"order"`
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

// RemoveStagePrefixes creates a slice with the build: or run: style prefixes removed.
func RemoveStagePrefixes(mixins []string) []string {
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

// FromSlice converts the given slice to a set in the form of unique keys in a map.
// The value associated with each key should not be relied upon. A value is present
// in the set if its key is present in the map, regardless of the key's value.
// from: https://github.com/buildpacks/pack/blob/main/internal/stringset/stringset.go
func FromSlice(strings []string) map[string]interface{} {
	set := map[string]interface{}{}
	for _, s := range strings {
		set[s] = nil
	}
	return set
}

// Compare performs a set comparison between two slices. `extra` represents elements present in
// `strings1` but not `strings2`. `missing` represents elements present in `strings2` that are
// missing from `strings1`. `common` represents elements present in both slices. Since the input
// slices are treated as sets, duplicates will be removed in any outputs.
// from: https://github.com/buildpacks/pack/blob/main/internal/stringset/stringset.go
func Compare(strings1, strings2 []string) (extra []string, missing []string, common []string) {
	set1 := FromSlice(strings1)
	set2 := FromSlice(strings2)

	for s := range set1 {
		if _, ok := set2[s]; !ok {
			extra = append(extra, s)
			continue
		}
		common = append(common, s)
	}

	for s := range set2 {
		if _, ok := set1[s]; !ok {
			missing = append(missing, s)
		}
	}

	return extra, missing, common
}

func syncLabels(sourceImg imgutil.Image, destImage imgutil.Image, test func(string) bool) error {
	if err := removeLabels(destImage, test); err != nil {
		return err
	}
	return copyLabels(sourceImg, destImage, test)
}

func removeLabels(image imgutil.Image, test func(string) bool) error {
	labels, err := image.Labels()
	if err != nil {
		return err
	}

	for label := range labels {
		if test(label) {
			if err := image.RemoveLabel(label); err != nil {
				return errors.Wrapf(err, "failed to remove label '%s'", label)
			}
		}
	}
	return nil
}

func copyLabels(fromImage imgutil.Image, destImage imgutil.Image, test func(string) bool) error {
	fromLabels, err := fromImage.Labels()
	if err != nil {
		return err
	}

	for label, labelValue := range fromLabels {
		if test(label) {
			if err := destImage.SetLabel(label, labelValue); err != nil {
				return err
			}
		}
	}
	return nil
}
