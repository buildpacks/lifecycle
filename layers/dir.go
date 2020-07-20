package layers

import (
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/archive"
)

// DirLayer creates a layer form the given directory
func (f *Factory) DirLayer(id string, dir string) (layer Layer, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return Layer{}, err
	}

	tarPath := filepath.Join(f.ArtifactsDir, escape(id)+".tar")
	if f.tarHashes == nil {
		f.tarHashes = make(map[string]string)
	}
	if sha, ok := f.tarHashes[tarPath]; ok {
		f.Logger.Debugf("Reusing tarball for layer %q with SHA: %s\n", id, sha)
		return Layer{
			ID:      id,
			TarPath: tarPath,
			Digest:  sha,
		}, nil
	}
	lw, err := newFileLayerWriter(tarPath)
	if err != nil {
		return Layer{}, err
	}
	defer func() {
		if closeErr := lw.Close(); err == nil {
			err = closeErr
		}
	}()
	tw := tarWriter(lw)
	parents, err := parents(dir)
	if err != nil {
		return Layer{}, err
	}
	if err := archive.WriteFilesToArchive(tw, parents); err != nil {
		return Layer{}, err
	}
	tw.WithUID(f.UID)
	tw.WithGID(f.GID)
	err = archive.AddDirToArchive(tw, dir)
	if err != nil {
		return Layer{}, errors.Wrapf(err, "exporting slice layer '%s'", id)
	}

	if err := tw.Close(); err != nil {
		return Layer{}, err
	}
	digest := lw.Digest()
	f.tarHashes[tarPath] = digest
	return Layer{
		ID:      id,
		Digest:  digest,
		TarPath: tarPath,
	}, err
}
