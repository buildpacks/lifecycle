package selective

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// TODO: test the functions in this file
// TODO: add docs comments for this file
func (l Path) SelectiveImage(h v1.Hash) (v1.Image, error) {
	ii, err := l.ImageIndex()
	if err != nil {
		return nil, err
	}

	// FIXME: we may want to re-implement partial.compressedImageExtender so that calling .Layers() on a selective image errors with a helpful message
	return ii.Image(h)
}
