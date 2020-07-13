package layers

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

type layerWriter struct {
	io.Writer
	io.Closer
	hasher *concurrentHasher
	path   string
}

func newFileLayerWriter(dest string) (*layerWriter, error) {
	hasher := newConcurrentHasher(sha256.New())
	file, err := os.Create(dest)
	if err != nil {
		return nil, err
	}
	w := io.MultiWriter(hasher, file)
	return &layerWriter{w, file, hasher, dest}, nil
}

func (lw *layerWriter) Digest() string {
	return fmt.Sprintf("sha256:%x", lw.hasher.Sum(nil))
}
