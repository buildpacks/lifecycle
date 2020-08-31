package layers

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"

	"github.com/buildpacks/lifecycle/archive"
)

func (f *Factory) SnapshotLayer(id string, snapshotFile string) (layer Layer, err error) {
	snapshotFile, err = filepath.Abs(snapshotFile)
	if err != nil {
		return Layer{}, err
	}

	return f.writeLayer(id, func(tw *archive.NormalizingTarWriter) error {
		data, err := os.Open(snapshotFile)
		if err != nil {
			return err
		}
		defer data.Close()

		tr := tar.NewReader(data)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break // End of archive
			}

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			if _, err := io.Copy(tw, tr); err != nil {
				return err
			}

			if err != nil {
				return err
			}
		}
		return nil
	})
}
