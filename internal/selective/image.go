package selective

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// TODO (before merging): add docs comment and tests
func (l Path) SelectiveImage(h v1.Hash) (v1.Image, error) {
	ii, err := l.ImageIndex()
	if err != nil {
		return nil, err
	}

	// TODO (before merging): re-implement partial.compressedImageExtender so that trying to access image layers errors with a helpful message
	return ii.Image(h)
}
