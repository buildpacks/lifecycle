package layers

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/archive"
)

type Factory struct {
	ArtifactsDir string
	UID, GID     int
	Logger       Logger

	tarHashes map[string]string // Stores hashes of layer tarballs for reuse between the export and cache steps.
}

type Layer struct {
	ID      string
	TarPath string
	Digest  string
}

type Logger interface {
	Debug(msg string)
	Debugf(fmt string, v ...interface{})

	Info(msg string)
	Infof(fmt string, v ...interface{})

	Warn(msg string)
	Warnf(fmt string, v ...interface{})

	Error(msg string)
	Errorf(fmt string, v ...interface{})
}

// DirLayer creates a layer form the given directory
func (f *Factory) DirLayer(id string, dir string) (Layer, error) {
	dir, err := filepath.Abs(dir)
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
		err = lw.Close()
	}()
	tw := archive.NewNormalizedTarWriter(tar.NewWriter(lw))
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
	return Layer{
		ID:      id,
		Digest:  lw.Digest(),
		TarPath: tarPath,
	}, err
}

func escape(id string) string {
	return strings.Replace(id, "/", "_", -1)
}

func parents(file string) ([]archive.PathInfo, error) {
	parent := filepath.Dir(file)
	if parent == "." || parent == "/" {
		return []archive.PathInfo{}, nil
	}
	fi, err := os.Stat(parent)
	if err != nil {
		return nil, err
	}
	parentDirs, err := parents(parent)
	if err != nil {
		return nil, err
	}
	return append(parentDirs, archive.PathInfo{
		Path: parent,
		Info: fi,
	}), nil
}
